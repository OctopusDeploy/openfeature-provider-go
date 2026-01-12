package openfeature

import (
	"context"
	"fmt"

	"github.com/open-feature/go-sdk/openfeature"
)

// OctopusProvider is the OpenFeature provider implementation for Octopus Deploy feature toggles.
type OctopusProvider struct {
	contextProvider *octopusContextProvider
	metadata        openfeature.Metadata
}

// NewOctopusProvider creates a new Octopus Deploy OpenFeature provider with the given configuration.
func NewOctopusProvider(config *OctopusConfiguration) *OctopusProvider {
	client := newOctopusClient(config.ServerURI, config.ClientIdentifier)
	contextProvider := newOctopusContextProvider(client, config.CacheDuration)

	return &OctopusProvider{
		contextProvider: contextProvider,
		metadata: openfeature.Metadata{
			Name: "octopus",
		},
	}
}

// Metadata returns the provider metadata.
func (p *OctopusProvider) Metadata() openfeature.Metadata {
	return p.metadata
}

// BooleanEvaluation evaluates a boolean feature flag.
// This is the only supported evaluation type for Octopus Deploy feature toggles.
func (p *OctopusProvider) BooleanEvaluation(
	ctx context.Context,
	flag string,
	defaultValue bool,
	evalContext openfeature.FlattenedContext,
) openfeature.BoolResolutionDetail {
	octopusCtx := p.contextProvider.getContext()
	return octopusCtx.evaluate(flag, defaultValue, evalContext)
}

// StringEvaluation is not supported by Octopus Deploy feature toggles.
func (p *OctopusProvider) StringEvaluation(
	ctx context.Context,
	flag string,
	defaultValue string,
	evalContext openfeature.FlattenedContext,
) openfeature.StringResolutionDetail {
	return openfeature.StringResolutionDetail{
		Value: defaultValue,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			ResolutionError: openfeature.NewTypeMismatchResolutionError("string evaluation not supported"),
			Reason:          openfeature.ErrorReason,
		},
	}
}

// FloatEvaluation is not supported by Octopus Deploy feature toggles.
func (p *OctopusProvider) FloatEvaluation(
	ctx context.Context,
	flag string,
	defaultValue float64,
	evalContext openfeature.FlattenedContext,
) openfeature.FloatResolutionDetail {
	return openfeature.FloatResolutionDetail{
		Value: defaultValue,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			ResolutionError: openfeature.NewTypeMismatchResolutionError("float evaluation not supported"),
			Reason:          openfeature.ErrorReason,
		},
	}
}

// IntEvaluation is not supported by Octopus Deploy feature toggles.
func (p *OctopusProvider) IntEvaluation(
	ctx context.Context,
	flag string,
	defaultValue int64,
	evalContext openfeature.FlattenedContext,
) openfeature.IntResolutionDetail {
	return openfeature.IntResolutionDetail{
		Value: defaultValue,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			ResolutionError: openfeature.NewTypeMismatchResolutionError("int evaluation not supported"),
			Reason:          openfeature.ErrorReason,
		},
	}
}

// ObjectEvaluation is not supported by Octopus Deploy feature toggles.
func (p *OctopusProvider) ObjectEvaluation(
	ctx context.Context,
	flag string,
	defaultValue interface{},
	evalContext openfeature.FlattenedContext,
) openfeature.InterfaceResolutionDetail {
	return openfeature.InterfaceResolutionDetail{
		Value: defaultValue,
		ProviderResolutionDetail: openfeature.ProviderResolutionDetail{
			ResolutionError: openfeature.NewTypeMismatchResolutionError("object evaluation not supported"),
			Reason:          openfeature.ErrorReason,
		},
	}
}

// Hooks returns the provider hooks (currently none).
func (p *OctopusProvider) Hooks() []openfeature.Hook {
	return []openfeature.Hook{}
}

// InitWithContext initializes the provider by fetching the initial feature toggles
// and starting the background refresh loop.
func (p *OctopusProvider) InitWithContext(ctx context.Context, evaluationContext openfeature.EvaluationContext) error {
	return p.contextProvider.initialize(ctx)
}

// ShutdownWithContext gracefully shuts down the provider by stopping the background refresh loop.
func (p *OctopusProvider) ShutdownWithContext(ctx context.Context) error {
	return p.contextProvider.shutdown(ctx)
}

// Init provides backward compatibility with the StateHandler interface.
func (p *OctopusProvider) Init(evaluationContext openfeature.EvaluationContext) error {
	return p.InitWithContext(context.Background(), evaluationContext)
}

// Shutdown provides backward compatibility with the StateHandler interface.
func (p *OctopusProvider) Shutdown() {
	if err := p.ShutdownWithContext(context.Background()); err != nil {
		fmt.Printf("Error during shutdown: %v\n", err)
	}
}
