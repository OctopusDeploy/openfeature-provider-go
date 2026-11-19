package octopus

import (
	"context"
	"strings"
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

	configuration.CacheDuration = time.Hour

	if provider.configuration.CacheDuration != time.Minute {
		t.Errorf("CacheDuration = %v, want the original value", provider.configuration.CacheDuration)
	}
}

func TestNewOctopusFeatureProviderRejectsNilConfiguration(t *testing.T) {
	if _, err := NewOctopusFeatureProvider(nil); err == nil {
		t.Error("expected an error for a nil configuration")
	}
}

func TestNewOctopusFeatureProviderRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(*OctopusFeatureConfiguration)
		wantErrs []string
	}{
		{"empty client identifier", func(c *OctopusFeatureConfiguration) { c.ClientIdentifier = "" }, []string{"ClientIdentifier"}},
		{"whitespace client identifier", func(c *OctopusFeatureConfiguration) { c.ClientIdentifier = "  \t" }, []string{"ClientIdentifier"}},
		{"zero-value product metadata", func(c *OctopusFeatureConfiguration) { c.ProductMetadata = ProductMetadata{} }, []string{"ProductMetadata"}},
		{"zero cache duration", func(c *OctopusFeatureConfiguration) { c.CacheDuration = 0 }, []string{"CacheDuration"}},
		{"negative cache duration", func(c *OctopusFeatureConfiguration) { c.CacheDuration = -time.Second }, []string{"CacheDuration"}},
		{"zero request timeout", func(c *OctopusFeatureConfiguration) { c.RequestTimeout = 0 }, []string{"RequestTimeout"}},
		{"negative request timeout", func(c *OctopusFeatureConfiguration) { c.RequestTimeout = -time.Second }, []string{"RequestTimeout"}},
		{
			"every problem at once",
			func(c *OctopusFeatureConfiguration) { *c = OctopusFeatureConfiguration{} },
			[]string{"ClientIdentifier", "ProductMetadata", "CacheDuration", "RequestTimeout"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := newTestConfiguration(t)
			test.modify(configuration)

			provider, err := NewOctopusFeatureProvider(configuration)
			if err == nil {
				t.Fatal("expected an error")
			}
			if provider != nil {
				t.Error("expected no provider alongside the error")
			}

			for _, want := range test.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
		})
	}
}

func TestNewOctopusFeatureProviderDefaultsServerUriForStructLiteralConfiguration(t *testing.T) {
	metadata, err := NewProductMetadata("test-product")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := NewOctopusFeatureProvider(&OctopusFeatureConfiguration{
		ClientIdentifier: "client-identifier",
		ProductMetadata:  metadata,
		CacheDuration:    time.Minute,
		RequestTimeout:   10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	if provider.configuration.serverUri != defaultServerUri {
		t.Errorf("serverUri = %q, want %q", provider.configuration.serverUri, defaultServerUri)
	}
}

func TestNewOctopusFeatureProviderAllowsRequestTimeoutLongerThanCacheDuration(t *testing.T) {
	configuration := newTestConfiguration(t)
	configuration.CacheDuration = time.Second
	configuration.RequestTimeout = time.Minute

	if _, err := NewOctopusFeatureProvider(configuration); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
