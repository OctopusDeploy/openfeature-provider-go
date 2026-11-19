package octopus

import (
	"context"
	"errors"

	"github.com/open-feature/go-sdk/openfeature"
)

const flagNotFoundMessage = "The slug provided did not match any of your Octopus Feature Flags. Please double check your slug and try again."

var (
	_ openfeature.FeatureProvider          = (*OctopusFeatureProvider)(nil)
	_ openfeature.ContextAwareStateHandler = (*OctopusFeatureProvider)(nil)
)

// OctopusFeatureProvider is an OpenFeature provider for Octopus feature
// flags. Create one with [NewOctopusFeatureProvider] and register it with the
// OpenFeature SDK.
type OctopusFeatureProvider struct {
	configuration OctopusFeatureConfiguration
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

	return provider, nil
}

// Metadata implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{Name: "octopus-go-provider"}
}

// Hooks implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) Hooks() []openfeature.Hook {
	return nil
}

// Init implements [openfeature.StateHandler].
func (p *OctopusFeatureProvider) Init(evaluationContext openfeature.EvaluationContext) error {
	return p.InitWithContext(context.Background(), evaluationContext)
}

// InitWithContext implements [openfeature.ContextAwareStateHandler].
func (p *OctopusFeatureProvider) InitWithContext(_ context.Context, _ openfeature.EvaluationContext) error {
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
func (p *OctopusFeatureProvider) BooleanEvaluation(_ context.Context, _ string, defaultValue bool, _ openfeature.FlattenedContext) openfeature.BoolResolutionDetail {
	return openfeature.BoolResolutionDetail{Value: defaultValue, ProviderResolutionDetail: flagNotFound()}
}

// StringEvaluation implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) StringEvaluation(_ context.Context, _ string, defaultValue string, _ openfeature.FlattenedContext) openfeature.StringResolutionDetail {
	return openfeature.StringResolutionDetail{Value: defaultValue, ProviderResolutionDetail: flagNotFound()}
}

// FloatEvaluation implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) FloatEvaluation(_ context.Context, _ string, defaultValue float64, _ openfeature.FlattenedContext) openfeature.FloatResolutionDetail {
	return openfeature.FloatResolutionDetail{Value: defaultValue, ProviderResolutionDetail: flagNotFound()}
}

// IntEvaluation implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) IntEvaluation(_ context.Context, _ string, defaultValue int64, _ openfeature.FlattenedContext) openfeature.IntResolutionDetail {
	return openfeature.IntResolutionDetail{Value: defaultValue, ProviderResolutionDetail: flagNotFound()}
}

// ObjectEvaluation implements [openfeature.FeatureProvider].
func (p *OctopusFeatureProvider) ObjectEvaluation(_ context.Context, _ string, defaultValue any, _ openfeature.FlattenedContext) openfeature.InterfaceResolutionDetail {
	return openfeature.InterfaceResolutionDetail{Value: defaultValue, ProviderResolutionDetail: flagNotFound()}
}

func flagNotFound() openfeature.ProviderResolutionDetail {
	return openfeature.ProviderResolutionDetail{
		ResolutionError: openfeature.NewFlagNotFoundResolutionError(flagNotFoundMessage),
		Reason:          openfeature.ErrorReason,
	}
}
