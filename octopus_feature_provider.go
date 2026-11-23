package octopus

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

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

	// Nil when the caller supplied HttpClient and therefore keeps ownership
	// of it.
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
		httpClient = &http.Client{Transport: newOwnedTransport()}
		provider.ownedHttpClient = httpClient
	}

	apiClient := featureflags.NewFeatureFlagApiClient(featureflags.FeatureFlagApiClientOptions{
		ServerUri:              provider.configuration.serverUri,
		ClientIdentifier:       provider.configuration.ClientIdentifier,
		OctopusClient:          octopusClientHeader(provider.configuration.ProductMetadata),
		ReleaseVersionOverride: provider.configuration.ReleaseVersionOverride,
		RequestTimeout:         provider.configuration.RequestTimeout,
		HttpClient:             httpClient,
	})
	provider.evaluatorCache = featureflags.NewFeatureFlagEvaluatorCache(apiClient, logger)

	return provider, nil
}

// newOwnedTransport clones the default transport so closing idle connections
// on shutdown can't affect anything else in the process that shares
// http.DefaultTransport.
func newOwnedTransport() http.RoundTripper {
	if transport, ok := http.DefaultTransport.(*http.Transport); ok {
		return transport.Clone()
	}

	// The application has wrapped the default transport, for example for
	// tracing, so start from net/http's defaults instead.
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func octopusClientHeader(productMetadata ProductMetadata) string {
	product := productMetadata.Name()
	if productMetadata.Version() != "" {
		product += "/" + productMetadata.Version()
	}

	return product + " openfeature-provider-go/" + version
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
// fetches the feature flags once, stopping early if ctx is done. It never
// returns an error: if the fetch fails, the provider is still ready, and
// every evaluation returns the default with FLAG_NOT_FOUND.
func (p *OctopusFeatureProvider) InitWithContext(ctx context.Context, _ openfeature.EvaluationContext) error {
	p.evaluatorCache.Initialize(ctx)

	return nil
}

// Shutdown implements [openfeature.StateHandler].
func (p *OctopusFeatureProvider) Shutdown() {
	_ = p.ShutdownWithContext(context.Background())
}

// ShutdownWithContext implements [openfeature.ContextAwareStateHandler].
func (p *OctopusFeatureProvider) ShutdownWithContext(_ context.Context) error {
	return nil
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
