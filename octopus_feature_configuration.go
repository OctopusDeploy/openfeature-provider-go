package octopus

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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

	// CacheDuration is how often the provider polls the feature flags service
	// for new feature flag evaluations. Defaults to 1 minute.
	CacheDuration time.Duration

	// RequestTimeout is how long a request to the feature flags service may
	// take before it is abandoned. It covers the whole request, connecting
	// included, and should be kept well under CacheDuration so a stalled
	// request can't hold up the next poll. Defaults to 10 seconds.
	RequestTimeout time.Duration

	// ReleaseVersionOverride overrides the application release version
	// embedded in ClientIdentifier. It is sent to the feature flags service
	// only when not empty.
	ReleaseVersionOverride string

	// Logger receives the provider's log output. Nil means the provider logs
	// nothing.
	Logger *slog.Logger

	// HttpClient is used for requests to the feature flags service, for
	// example to configure a proxy, TLS or instrumentation. Nil means the
	// provider creates and owns its own client. The provider never closes a
	// client set here.
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

func (c *OctopusFeatureConfiguration) validate() error {
	var errs []error

	if strings.TrimSpace(c.ClientIdentifier) == "" {
		errs = append(errs, errors.New("ClientIdentifier must not be empty"))
	}
	// The constructors guarantee a name, so an empty one means a zero value.
	if c.ProductMetadata.Name() == "" {
		errs = append(errs, errors.New("ProductMetadata must be created with NewProductMetadata or NewProductMetadataWithVersion"))
	}
	if c.CacheDuration <= 0 {
		errs = append(errs, fmt.Errorf("CacheDuration must be positive, got %v", c.CacheDuration))
	}
	if c.RequestTimeout <= 0 {
		errs = append(errs, fmt.Errorf("RequestTimeout must be positive, got %v", c.RequestTimeout))
	}

	return errors.Join(errs...)
}
