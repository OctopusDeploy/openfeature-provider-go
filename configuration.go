package openfeature

import (
	"time"
)

const (
	// DefaultServerURI is the default Octopus Deploy feature toggle service URL.
	DefaultServerURI = "https://features.octopus.com"

	// DefaultCacheDuration is the default interval between feature toggle refresh checks.
	DefaultCacheDuration = 1 * time.Minute
)

// OctopusConfiguration holds the configuration for the Octopus Deploy OpenFeature provider.
type OctopusConfiguration struct {
	// ClientIdentifier is the JWT token or client identifier for authenticating with the Octopus service.
	// This is typically obtained from the Octopus Deploy environment.
	ClientIdentifier string

	// ServerURI is the URL of the Octopus Deploy feature toggle service.
	// Defaults to https://features.octopus.com
	ServerURI string

	// CacheDuration is the interval between checks for feature toggle changes.
	// Defaults to 1 minute.
	CacheDuration time.Duration
}

// NewOctopusConfiguration creates a new configuration with the given client identifier
// and sensible defaults for other settings.
func NewOctopusConfiguration(clientIdentifier string) *OctopusConfiguration {
	return &OctopusConfiguration{
		ClientIdentifier: clientIdentifier,
		ServerURI:        DefaultServerURI,
		CacheDuration:    DefaultCacheDuration,
	}
}

// WithServerURI sets a custom server URI for the configuration.
func (c *OctopusConfiguration) WithServerURI(serverURI string) *OctopusConfiguration {
	c.ServerURI = serverURI
	return c
}

// WithCacheDuration sets a custom cache duration for the configuration.
func (c *OctopusConfiguration) WithCacheDuration(duration time.Duration) *OctopusConfiguration {
	c.CacheDuration = duration
	return c
}
