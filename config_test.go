package octopus

import (
	"testing"
	"time"
)

func TestNewConfigSetsDefaults(t *testing.T) {
	configuration := newTestConfiguration(t)

	if configuration.CacheDuration != time.Minute {
		t.Errorf("CacheDuration = %v, want %v", configuration.CacheDuration, time.Minute)
	}
	if configuration.RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want %v", configuration.RequestTimeout, 10*time.Second)
	}
	if configuration.serverURI != "https://features.octopus.com" {
		t.Errorf("serverURI = %q, want %q", configuration.serverURI, "https://features.octopus.com")
	}
}
