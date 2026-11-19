package featureflags

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
)

// FeatureFlagEvaluatorCache holds the evaluator the provider evaluates
// against. It starts with the empty evaluator, so evaluations return
// FLAG_NOT_FOUND until a manifest has been fetched.
type FeatureFlagEvaluatorCache struct {
	client    *FeatureFlagApiClient
	logger    *slog.Logger
	evaluator atomic.Pointer[FeatureFlagEvaluator]
}

// NewFeatureFlagEvaluatorCache returns a cache that fetches with client. The
// logger must not be nil.
func NewFeatureFlagEvaluatorCache(client *FeatureFlagApiClient, logger *slog.Logger) *FeatureFlagEvaluatorCache {
	cache := &FeatureFlagEvaluatorCache{client: client, logger: logger}
	cache.evaluator.Store(EmptyFeatureFlagEvaluator(logger))

	return cache
}

// GetEvaluator returns the current evaluator.
func (c *FeatureFlagEvaluatorCache) GetEvaluator() *FeatureFlagEvaluator {
	return c.evaluator.Load()
}

// Initialize fetches the manifest once. A failed fetch is logged, and the
// cache keeps serving the empty evaluator.
func (c *FeatureFlagEvaluatorCache) Initialize(ctx context.Context) {
	response, err := c.client.GetServerSideEvaluations(ctx)
	if err != nil {
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
