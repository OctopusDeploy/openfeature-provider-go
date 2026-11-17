package octopus

import (
	"testing"
	"time"
)

func TestNewOctopusFeatureConfigurationSetsDefaults(t *testing.T) {
	metadata, err := NewProductMetadataWithVersion("test-product", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}

	configuration := NewOctopusFeatureConfiguration("client-identifier", metadata)

	if configuration.ClientIdentifier != "client-identifier" {
		t.Errorf("ClientIdentifier = %q, want %q", configuration.ClientIdentifier, "client-identifier")
	}
	if configuration.ProductMetadata != metadata {
		t.Errorf("ProductMetadata = %+v, want %+v", configuration.ProductMetadata, metadata)
	}
	if configuration.CacheDuration != time.Minute {
		t.Errorf("CacheDuration = %v, want %v", configuration.CacheDuration, time.Minute)
	}
	if configuration.RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want %v", configuration.RequestTimeout, 10*time.Second)
	}
	if configuration.ReleaseVersionOverride != "" {
		t.Errorf("ReleaseVersionOverride = %q, want empty", configuration.ReleaseVersionOverride)
	}
	if configuration.Logger != nil {
		t.Error("Logger is set, want nil")
	}
	if configuration.HttpClient != nil {
		t.Error("HttpClient is set, want nil")
	}
	if configuration.serverUri != "https://features.octopus.com" {
		t.Errorf("serverUri = %q, want %q", configuration.serverUri, "https://features.octopus.com")
	}
}
