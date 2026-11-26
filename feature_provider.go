package octopus

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/OctopusDeploy/openfeature-provider-go/internal/featureflags"
	"github.com/open-feature/go-sdk/openfeature"
)

const (
	providerName        = "octopus-go-provider"
	flagNotFoundMessage = "The slug provided did not match any of your Octopus Feature Flags. Please double check your slug and try again."
)

var (
	_ openfeature.FeatureProvider          = (*FeatureProvider)(nil)
	_ openfeature.ContextAwareStateHandler = (*FeatureProvider)(nil)
)

// FeatureProvider is an OpenFeature provider for Octopus feature flags.
// Create one with [NewFeatureProvider] and register it with the OpenFeature
// SDK.
type FeatureProvider struct {
	configuration  FeatureConfiguration
	evaluatorCache *featureflags.FeatureFlagEvaluatorCache

	// Nil when the provider doesn't own the client's transport: the caller
	// supplied HTTPClient, or the application has wrapped
	// http.DefaultTransport.
	ownedHTTPClient *http.Client
}

// NewFeatureProvider returns a provider for the given configuration.
// It returns an error if ClientIdentifier is empty, ClientIdentifier or
// ReleaseVersionOverride contains a line break or other control character,
// ProductMetadata wasn't created with one of its constructors, CacheDuration
// or RequestTimeout isn't positive, or ServerURI is set but isn't an absolute
// http or https URI. The provider keeps its own copy, so changes to
// configuration afterwards have no effect on it. It makes no requests until
// the SDK initializes it.
func NewFeatureProvider(configuration *FeatureConfiguration) (*FeatureProvider, error) {
	if configuration == nil {
		return nil, errors.New("configuration must not be nil")
	}
	if err := configuration.validate(); err != nil {
		return nil, err
	}

	provider := &FeatureProvider{configuration: *configuration}
	// A configuration built as a struct literal skips the constructor's default.
	if provider.configuration.ServerURI == "" {
		provider.configuration.ServerURI = defaultServerURI
	}

	logger := provider.configuration.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	logger = logger.With(slog.String("provider", providerName))

	httpClient := provider.configuration.HTTPClient
	if httpClient == nil {
		var owned bool
		httpClient, owned = newDefaultHTTPClient(http.DefaultTransport)
		if owned {
			provider.ownedHTTPClient = httpClient
		}
	}

	apiClient := featureflags.NewFeatureFlagApiClient(featureflags.FeatureFlagApiClientOptions{
		ServerUri:              provider.configuration.ServerURI,
		ClientIdentifier:       provider.configuration.ClientIdentifier,
		ProductName:            provider.configuration.ProductMetadata.Name(),
		ProductVersion:         provider.configuration.ProductMetadata.Version(),
		ProviderVersion:        version,
		ReleaseVersionOverride: provider.configuration.ReleaseVersionOverride,
		RequestTimeout:         provider.configuration.RequestTimeout,
		HttpClient:             httpClient,
	})
	provider.evaluatorCache = featureflags.NewFeatureFlagEvaluatorCache(apiClient, provider.configuration.CacheDuration, logger)

	return provider, nil
}

// newDefaultHTTPClient returns the client used when none is configured. A
// plain transport is cloned, so closing its idle connections on shutdown
// can't affect anything else that shares it. A wrapped one, for example for
// tracing, is used as-is so requests still go through the wrapper, and isn't
// owned.
func newDefaultHTTPClient(base http.RoundTripper) (client *http.Client, owned bool) {
	if transport, ok := base.(*http.Transport); ok {
		return &http.Client{Transport: transport.Clone()}, true
	}

	return &http.Client{Transport: base}, false
}

// Metadata implements [openfeature.FeatureProvider].
func (p *FeatureProvider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{Name: providerName}
}

// Hooks implements [openfeature.FeatureProvider].
func (p *FeatureProvider) Hooks() []openfeature.Hook {
	return nil
}

// Init implements [openfeature.StateHandler].
func (p *FeatureProvider) Init(evaluationContext openfeature.EvaluationContext) error {
	return p.InitWithContext(context.Background(), evaluationContext)
}

// InitWithContext implements [openfeature.ContextAwareStateHandler]. It
// fetches the feature flags, stopping early if ctx is done, and then checks
// for changes every CacheDuration until shutdown. A failed fetch isn't an
// error: the provider is still ready, and every evaluation returns the
// default with FLAG_NOT_FOUND until a later check succeeds. Calling it again
// while the provider is running waits for the first fetch and starts nothing
// new. After shutdown it returns an error, because a provider can't be
// initialized again.
func (p *FeatureProvider) InitWithContext(ctx context.Context, _ openfeature.EvaluationContext) error {
	return p.evaluatorCache.Start(ctx)
}

// Shutdown implements [openfeature.StateHandler].
func (p *FeatureProvider) Shutdown() {
	_ = p.ShutdownWithContext(context.Background())
}

// ShutdownWithContext implements [openfeature.ContextAwareStateHandler]. It
// stops checking for changes and cancels any request in flight, even if ctx
// is already done, then waits for that to finish, returning ctx's error if
// ctx ends first. Evaluations keep using the last feature flags fetched. It
// can be called more than once, and the provider can't be initialized again
// afterwards.
func (p *FeatureProvider) ShutdownWithContext(ctx context.Context) error {
	err := p.evaluatorCache.Shutdown(ctx)

	if p.ownedHTTPClient != nil {
		p.ownedHTTPClient.CloseIdleConnections()
	}

	return err
}

// BooleanEvaluation implements [openfeature.FeatureProvider].
func (p *FeatureProvider) BooleanEvaluation(_ context.Context, flag string, defaultValue bool, flatCtx openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	return p.evaluatorCache.GetEvaluator().Evaluate(flag, defaultValue, flatCtx)
}

// StringEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *FeatureProvider) StringEvaluation(_ context.Context, flag string, defaultValue string, _ openfeature.FlattenedContext) openfeature.StringResolutionDetail {
	return openfeature.StringResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

// FloatEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *FeatureProvider) FloatEvaluation(_ context.Context, flag string, defaultValue float64, _ openfeature.FlattenedContext) openfeature.FloatResolutionDetail {
	return openfeature.FloatResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

// IntEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *FeatureProvider) IntEvaluation(_ context.Context, flag string, defaultValue int64, _ openfeature.FlattenedContext) openfeature.IntResolutionDetail {
	return openfeature.IntResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

// ObjectEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *FeatureProvider) ObjectEvaluation(_ context.Context, flag string, defaultValue any, _ openfeature.FlattenedContext) openfeature.InterfaceResolutionDetail {
	return openfeature.InterfaceResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

func (p *FeatureProvider) rejectNonBooleanEvaluation(flag string) openfeature.ProviderResolutionDetail {
	err := openfeature.NewTypeMismatchResolutionError("Octopus only supports boolean flags.")
	if _, found := p.evaluatorCache.GetEvaluator().FindEvaluationBySlug(flag); !found {
		err = openfeature.NewFlagNotFoundResolutionError(flagNotFoundMessage)
	}

	return openfeature.ProviderResolutionDetail{ResolutionError: err, Reason: openfeature.ErrorReason}
}
