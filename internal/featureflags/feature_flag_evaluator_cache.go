package featureflags

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// FeatureFlagEvaluatorCache holds the evaluator the provider evaluates
// against, and keeps it current by polling for changes. It starts with the
// empty evaluator, so evaluations return FLAG_NOT_FOUND until a manifest has
// been fetched.
type FeatureFlagEvaluatorCache struct {
	client        *FeatureFlagApiClient
	cacheDuration time.Duration
	logger        *slog.Logger
	evaluator     atomic.Pointer[FeatureFlagEvaluator]

	// The lifecycle runs one way: Start, then Shutdown. These are created
	// once, so every caller waits on the same channels.
	runCtx      context.Context
	stop        context.CancelFunc
	initialized chan struct{} // closed once the first fetch has finished
	done        chan struct{} // closed once the poller has exited

	mu      sync.Mutex
	started bool
	closed  bool
}

// ErrShutDown is returned by [FeatureFlagEvaluatorCache.Start] once the
// cache has been shut down.
var ErrShutDown = errors.New("the Octopus feature provider has been shut down and can't be initialized again; create a new provider instead")

// NewFeatureFlagEvaluatorCache returns a cache that fetches with client and
// checks for changes every cacheDuration. The logger must not be nil.
func NewFeatureFlagEvaluatorCache(client *FeatureFlagApiClient, cacheDuration time.Duration, logger *slog.Logger) *FeatureFlagEvaluatorCache {
	runCtx, stop := context.WithCancel(context.Background())
	cache := &FeatureFlagEvaluatorCache{
		client:        client,
		cacheDuration: cacheDuration,
		logger:        logger,
		runCtx:        runCtx,
		stop:          stop,
		initialized:   make(chan struct{}),
		done:          make(chan struct{}),
	}
	cache.evaluator.Store(EmptyFeatureFlagEvaluator(logger))

	return cache
}

// GetEvaluator returns the current evaluator.
func (c *FeatureFlagEvaluatorCache) GetEvaluator() *FeatureFlagEvaluator {
	return c.evaluator.Load()
}

// Start fetches the manifest once, then polls for changes until Shutdown,
// whether or not that fetch succeeded. A failed fetch is logged, and the
// cache keeps serving the evaluator it has. Calling it again while the cache
// is running waits for the first fetch to finish, or for ctx to end, and
// starts nothing new. After Shutdown it returns [ErrShutDown].
func (c *FeatureFlagEvaluatorCache) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrShutDown
	}
	if c.started {
		c.mu.Unlock()
		select {
		case <-c.initialized:
		case <-ctx.Done():
		}
		return nil
	}
	c.started = true
	c.mu.Unlock()

	// The SDK can shut a provider down while it's still initializing, when
	// it's replaced, so shutdown cancels this fetch as well.
	fetchCtx, cancelFetch := context.WithCancel(ctx)
	stopCancelling := context.AfterFunc(c.runCtx, cancelFetch)
	c.initialize(fetchCtx)
	stopCancelling()
	cancelFetch()
	close(c.initialized)

	go func() {
		defer close(c.done)
		c.poll(c.runCtx)
	}()

	return nil
}

// Shutdown stops polling and cancels any request in flight, even if ctx is
// already done, and stops Start from running again. It then waits for the
// poller to exit, returning ctx's error if ctx ends first. It can be called
// any number of times, including before Start. The cache keeps serving its
// last evaluator.
func (c *FeatureFlagEvaluatorCache) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	c.closed = true
	started := c.started
	c.mu.Unlock()

	c.stop()
	if !started {
		return nil
	}

	// Checked first so an already-cancelled ctx doesn't hide a poller that
	// has already exited.
	select {
	case <-c.done:
		return nil
	default:
	}

	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *FeatureFlagEvaluatorCache) initialize(ctx context.Context) {
	response, err := c.client.GetServerSideEvaluations(ctx)
	if err != nil {
		// Shutdown cancelled the fetch, so there's nothing to report.
		if c.runCtx.Err() != nil {
			return
		}

		var rejected *RejectedResponseError
		if errors.As(err, &rejected) {
			c.logger.LogAttrs(ctx, slog.LevelWarn, rejected.Message, rejected.Attrs...)
		} else {
			c.logger.Error("Failed to retrieve feature manifest during initialization. Falling back to no evaluations, defaults will be used during evaluation.", slog.Any("error", err))
		}

		return
	}

	c.evaluator.Store(NewFeatureFlagEvaluator(response, c.logger))
}

func (c *FeatureFlagEvaluatorCache) poll(ctx context.Context) {
	// A timer reset after each refresh, rather than a ticker, so a slow
	// refresh can't be followed immediately by another.
	timer := time.NewTimer(c.cacheDuration)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		c.refresh(ctx)
		timer.Reset(c.cacheDuration)
	}
}

func (c *FeatureFlagEvaluatorCache) refresh(ctx context.Context) {
	err := c.fetchIfChanged(ctx)
	// A cancelled ctx is shutdown. A request that hit RequestTimeout fails
	// with its own deadline and is reported.
	if err == nil || ctx.Err() != nil {
		return
	}

	attrs := []slog.Attr{slog.Any("error", err)}
	var rejected *RejectedResponseError
	if errors.As(err, &rejected) {
		attrs = append(attrs, rejected.Attrs...)
	}
	c.logger.LogAttrs(ctx, slog.LevelError, "Failed to retrieve updated feature manifest. Retaining the existing evaluations, which may be stale.", attrs...)
}

func (c *FeatureFlagEvaluatorCache) fetchIfChanged(ctx context.Context) error {
	changed, err := c.client.HaveFeaturesChanged(ctx, c.GetEvaluator().ContentHash())
	if err != nil || !changed {
		return err
	}

	response, err := c.client.GetServerSideEvaluations(ctx)
	if err != nil {
		return err
	}

	c.evaluator.Store(NewFeatureFlagEvaluator(response, c.logger))

	return nil
}
