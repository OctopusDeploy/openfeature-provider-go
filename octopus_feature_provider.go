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
	_ openfeature.FeatureProvider          = (*OctopusFeatureProvider)(nil)
	_ openfeature.ContextAwareStateHandler = (*OctopusFeatureProvider)(nil)
)

// OctopusFeatureProvider is an OpenFeature provider for Octopus feature
// flags. Create one with [NewOctopusFeatureProvider] and register it with the
// OpenFeature SDK.
type OctopusFeatureProvider struct {
	configuration  OctopusFeatureConfiguration
	evaluatorCache *featureflags.FeatureFlagEvaluatorCache

	// Nil when the provider doesn't own the client's transport: the caller
	// supplied HttpClient, or the application has wrapped
	// http.DefaultTransport.
	ownedHttpClient *http.Client
}

// NewOctopusFeatureProvider returns a provider for the given configuration.
// It returns an error if ClientIdentifier is empty, ProductMetadata wasn't
// created with one of its constructors, or CacheDuration or RequestTimeout
// isn't positive. The provider keeps its own copy, so changes to
// configuration afterwards have no effect on it. It makes no requests until
// the SDK initializes it.
func NewOctopusFeatureProvider(configuration *OctopusFeatureConfiguration) (*OctopusFeatureProvider, error) {
	if configuration == nil {
		return nil, errors.New("configuration must not be nil")
	}
	if err := configuration.validate(); err != nil {
		return nil, err
	}

	provider := &OctopusFeatureProvider{configuration: *configuration}
	// A configuration built as a struct literal skips the constructor's default.
	if provider.configuration.serverUri == "" {
		provider.configuration.serverUri = defaultServerUri
	}

	logger := provider.configuration.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	logger = logger.With(slog.String("provider", providerName))

	httpClient := provider.configuration.HttpClient
	if httpClient == nil {
		var owned bool
		httpClient, owned = newDefaultHttpClient(http.DefaultTransport)
		if owned {
			provider.ownedHttpClient = httpClient
		}
	}

	apiClient := featureflags.NewFeatureFlagApiClient(featureflags.FeatureFlagApiClientOptions{
		ServerUri:              provider.configuration.serverUri,
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

// newDefaultHttpClient returns the client used when none is configured. A
// plain transport is cloned, so closing its idle connections on shutdown
// can't affect anything else that shares it. A wrapped one, for example for
// tracing, is used as-is so requests still go through the wrapper, and isn't
// owned.
func newDefaultHttpClient(base http.RoundTripper) (client *http.Client, owned bool) {
	if transport, ok := base.(*http.Transport); ok {
		return &http.Client{Transport: transport.Clone()}, true
	}

	return &http.Client{Transport: base}, false
}

// Metadata implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{Name: providerName}
}

// Hooks implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) Hooks() []openfeature.Hook {
	return nil
}

// Init implements [openfeature.StateHandler].
func (p *OctopusFeatureProvider) Init(evaluationContext openfeature.EvaluationContext) error {
	return p.InitWithContext(context.Background(), evaluationContext)
}

// InitWithContext implements [openfeature.ContextAwareStateHandler]. It
// fetches the feature flags once, stopping early if ctx is done, and then
// checks for changes every CacheDuration until shutdown. It never returns an
// error: if the fetch fails, the provider is still ready, and every
// evaluation returns the default with FLAG_NOT_FOUND until a later check
// succeeds. Calling it again while the provider is running does nothing.
func (p *OctopusFeatureProvider) InitWithContext(ctx context.Context, _ openfeature.EvaluationContext) error {
	p.evaluatorCache.Start(ctx)

	return nil
}

// Shutdown implements [openfeature.StateHandler].
func (p *OctopusFeatureProvider) Shutdown() {
	_ = p.ShutdownWithContext(context.Background())
}

// ShutdownWithContext implements [openfeature.ContextAwareStateHandler]. It
// stops checking for changes and cancels any request in flight, even if ctx
// is already done, and uses ctx only to bound waiting for that to finish.
// Evaluations keep using the last feature flags fetched. Calling it again
// does nothing, and the provider can be initialized again afterwards.
func (p *OctopusFeatureProvider) ShutdownWithContext(ctx context.Context) error {
	err := p.evaluatorCache.Shutdown(ctx)

	if p.ownedHttpClient != nil {
		p.ownedHttpClient.CloseIdleConnections()
	}

	return err
}

// BooleanEvaluation implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) BooleanEvaluation(_ context.Context, flag string, defaultValue bool, flatCtx openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	return p.evaluatorCache.GetEvaluator().Evaluate(flag, defaultValue, flatCtx)
}

// StringEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *OctopusFeatureProvider) StringEvaluation(_ context.Context, flag string, defaultValue string, _ openfeature.FlattenedContext) openfeature.StringResolutionDetail {
	return openfeature.StringResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

// FloatEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *OctopusFeatureProvider) FloatEvaluation(_ context.Context, flag string, defaultValue float64, _ openfeature.FlattenedContext) openfeature.FloatResolutionDetail {
	return openfeature.FloatResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

// IntEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *OctopusFeatureProvider) IntEvaluation(_ context.Context, flag string, defaultValue int64, _ openfeature.FlattenedContext) openfeature.IntResolutionDetail {
	return openfeature.IntResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

// ObjectEvaluation implements [openfeature.FeatureProvider]. Octopus only
// supports boolean flags, so it always returns the default.
func (p *OctopusFeatureProvider) ObjectEvaluation(_ context.Context, flag string, defaultValue any, _ openfeature.FlattenedContext) openfeature.InterfaceResolutionDetail {
	return openfeature.InterfaceResolutionDetail{Value: defaultValue, ProviderResolutionDetail: p.rejectNonBooleanEvaluation(flag)}
}

func (p *OctopusFeatureProvider) rejectNonBooleanEvaluation(flag string) openfeature.ProviderResolutionDetail {
	err := openfeature.NewTypeMismatchResolutionError("Octopus only supports boolean flags.")
	if _, found := p.evaluatorCache.GetEvaluator().FindEvaluationBySlug(flag); !found {
		err = openfeature.NewFlagNotFoundResolutionError(flagNotFoundMessage)
	}

	return openfeature.ProviderResolutionDetail{ResolutionError: err, Reason: openfeature.ErrorReason}
}
