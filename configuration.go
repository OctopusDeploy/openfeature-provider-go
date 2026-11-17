package octopus

import (
	"log/slog"
	"net/http"
	"time"
)

const defaultServerUri = "https://features.octopus.com"

// OctopusFeatureConfiguration configures an [OctopusFeatureProvider]. Create
// one with [NewOctopusFeatureConfiguration], then assign any optional fields
// before passing it to [NewOctopusFeatureProvider].
type OctopusFeatureConfiguration struct {
	// ClientIdentifier is the value of the Octopus variable
	// Octopus.FeatureFlags.ClientIdentifier.
	ClientIdentifier string

	// ProductMetadata describes the product using the provider.
	ProductMetadata ProductMetadata

	// CacheDuration is how often the provider polls Octopus for new feature
	// flag evaluations. Defaults to 1 minute.
	CacheDuration time.Duration

	// RequestTimeout is how long a request to Octopus may take before it is
	// abandoned. It covers the whole request, connecting included, and should
	// be kept well under CacheDuration so a stalled request can't hold up the
	// next poll. Defaults to 10 seconds.
	RequestTimeout time.Duration

	// ReleaseVersionOverride overrides the application release version
	// embedded in ClientIdentifier. It is sent to Octopus only when not empty.
	ReleaseVersionOverride string

	// Logger receives the provider's log output. Nil means the provider logs
	// nothing.
	Logger *slog.Logger

	// HttpClient is used for requests to Octopus, for example to configure a
	// proxy, TLS or instrumentation. Nil means the provider creates and owns
	// its own client. The provider never closes a client set here.
	HttpClient *http.Client

	serverUri string
}

// NewOctopusFeatureConfiguration returns a configuration with the required
// values set and every optional field at its default.
func NewOctopusFeatureConfiguration(clientIdentifier string, productMetadata ProductMetadata) *OctopusFeatureConfiguration {
	return &OctopusFeatureConfiguration{
		ClientIdentifier: clientIdentifier,
		ProductMetadata:  productMetadata,
		CacheDuration:    time.Minute,
		RequestTimeout:   10 * time.Second,
		serverUri:        defaultServerUri,
	}
}
