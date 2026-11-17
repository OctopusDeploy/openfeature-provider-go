package octopus

import (
	"context"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

func newTestConfiguration(t *testing.T) *OctopusFeatureConfiguration {
	t.Helper()

	metadata, err := NewProductMetadata("test-product")
	if err != nil {
		t.Fatal(err)
	}

	return NewOctopusFeatureConfiguration("client-identifier", metadata)
}

func TestBooleanEvaluationThroughTheSdkReturnsDefaultWithFlagNotFound(t *testing.T) {
	provider, err := NewOctopusFeatureProvider(newTestConfiguration(t))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	api := isolated.NewAPI()
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = api.Shutdown(ctx) })

	client := api.NewClient()
	if state := client.State(); state != openfeature.ReadyState {
		t.Errorf("State() = %v, want %v", state, openfeature.ReadyState)
	}

	details, err := client.BooleanValueDetails(ctx, "some-flag", true, openfeature.EvaluationContext{})

	if err == nil {
		t.Error("expected an error")
	}
	if !details.Value {
		t.Error("Value = false, want the default true")
	}
	if details.ErrorCode != openfeature.FlagNotFoundCode {
		t.Errorf("ErrorCode = %v, want %v", details.ErrorCode, openfeature.FlagNotFoundCode)
	}
}

func TestProviderIsUnaffectedByLaterConfigurationChanges(t *testing.T) {
	configuration := newTestConfiguration(t)
	provider, err := NewOctopusFeatureProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}

	configuration.ClientIdentifier = "changed"
	configuration.CacheDuration = time.Hour

	if provider.configuration.ClientIdentifier != "client-identifier" {
		t.Errorf("ClientIdentifier = %q, want the original value", provider.configuration.ClientIdentifier)
	}
	if provider.configuration.CacheDuration != time.Minute {
		t.Errorf("CacheDuration = %v, want the original value", provider.configuration.CacheDuration)
	}
}

func TestNewOctopusFeatureProviderRejectsNilConfiguration(t *testing.T) {
	if _, err := NewOctopusFeatureProvider(nil); err == nil {
		t.Error("expected an error for a nil configuration")
	}
}
