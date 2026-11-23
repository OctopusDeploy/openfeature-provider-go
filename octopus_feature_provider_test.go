package octopus

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

const testManifest = `[{"slug": "enabled-flag", "value": true, "reason": "The flag is enabled for this environment."}]`

func newTestConfiguration(t *testing.T) *OctopusFeatureConfiguration {
	t.Helper()

	metadata, err := NewProductMetadata("test-product")
	if err != nil {
		t.Fatal(err)
	}

	return NewOctopusFeatureConfiguration("client-identifier", metadata)
}

// startProvider registers a provider for configuration with an isolated SDK
// API, waits for it to initialize, and returns a client bound to it.
func startProvider(t *testing.T, configuration *OctopusFeatureConfiguration) *openfeature.Client {
	t.Helper()

	provider, err := NewOctopusFeatureProvider(configuration)
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

	return client
}

func TestBooleanEvaluationReturnsTheServersValueAndReason(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
	client := startProvider(t, server.configuration(t))

	details, err := client.BooleanValueDetails(context.Background(), "enabled-flag", false, openfeature.EvaluationContext{})

	if err != nil {
		t.Fatal(err)
	}
	if !details.Value {
		t.Error("Value = false, want true")
	}
	if details.Reason != "The flag is enabled for this environment." {
		t.Errorf("Reason = %q, want the server's reason", details.Reason)
	}
}

func TestInitFetchesTheEvaluationsWithTheExpectedHeaders(t *testing.T) {
	versionedMetadata, err := NewProductMetadataWithVersion("test-product", "2024.1.0")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name                   string
		productMetadata        *ProductMetadata
		releaseVersionOverride string
		wantOctopusClient      string
		wantReleaseVersion     []string
	}{
		{
			name:              "product name only",
			wantOctopusClient: "test-product openfeature-provider-go/" + version,
		},
		{
			name:                   "product version and release version override",
			productMetadata:        &versionedMetadata,
			releaseVersionOverride: "1.2.3",
			wantOctopusClient:      "test-product/2024.1.0 openfeature-provider-go/" + version,
			wantReleaseVersion:     []string{"1.2.3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeFeatureFlagsServer(t)
			configuration := server.configuration(t)
			if tt.productMetadata != nil {
				configuration.ProductMetadata = *tt.productMetadata
			}
			configuration.ReleaseVersionOverride = tt.releaseVersionOverride

			startProvider(t, configuration)

			requests := server.recordedRequests()
			if len(requests) != 1 {
				t.Fatalf("got %d requests, want 1: %+v", len(requests), requests)
			}
			request := requests[0]
			if request.method != http.MethodGet || request.path != evaluationsPath {
				t.Errorf("request = %s %s, want GET %s", request.method, request.path, evaluationsPath)
			}
			if got := request.header.Get("Authorization"); got != "Bearer client-identifier" {
				t.Errorf("Authorization = %q, want %q", got, "Bearer client-identifier")
			}
			if got := request.header.Get("X-Octopus-Client"); got != tt.wantOctopusClient {
				t.Errorf("X-Octopus-Client = %q, want %q", got, tt.wantOctopusClient)
			}
			if got := request.header.Values("X-Release-Version"); !slices.Equal(got, tt.wantReleaseVersion) {
				t.Errorf("X-Release-Version = %q, want %q", got, tt.wantReleaseVersion)
			}
		})
	}
}

func TestInitFailureFallsBackToFlagNotFound(t *testing.T) {
	tests := []struct {
		name     string
		response fakeResponse
		// unreachable points the provider at a server that has shut down.
		unreachable bool
	}{
		{name: "not found", response: fakeResponse{status: http.StatusNotFound, body: testManifest, contentHash: []byte("hash")}},
		{name: "server error", response: fakeResponse{status: http.StatusInternalServerError, body: testManifest, contentHash: []byte("hash")}},
		{name: "unauthorized", response: fakeResponse{status: http.StatusUnauthorized, body: testManifest, contentHash: []byte("hash")}},
		{name: "missing content hash", response: fakeResponse{body: testManifest}},
		{name: "empty content hash", response: fakeResponse{body: testManifest, contentHash: []byte{}}},
		{name: "null body", response: fakeResponse{body: `null`, contentHash: []byte("hash")}},
		{name: "malformed body", response: fakeResponse{body: `{`, contentHash: []byte("hash")}},
		{name: "request timeout", response: fakeResponse{body: testManifest, contentHash: []byte("hash"), delay: time.Minute}},
		{name: "unreachable server", unreachable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeFeatureFlagsServer(t)
			server.setEvaluations(tt.response)
			// Logger stays nil, so this also catches a nil logger reaching the
			// evaluator when it warns about the missing slug.
			configuration := server.configuration(t)
			configuration.RequestTimeout = 100 * time.Millisecond
			if tt.unreachable {
				server.Close()
			}

			client := startProvider(t, configuration)

			details, err := client.BooleanValueDetails(context.Background(), "enabled-flag", true, openfeature.EvaluationContext{})

			if err == nil {
				t.Error("expected an error")
			}
			if !details.Value {
				t.Error("Value = false, want the default true")
			}
			if details.ErrorCode != openfeature.FlagNotFoundCode {
				t.Errorf("ErrorCode = %v, want %v", details.ErrorCode, openfeature.FlagNotFoundCode)
			}
		})
	}
}

func TestInitWithACancelledContextFallsBackToFlagNotFound(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
	provider, err := NewOctopusFeatureProvider(server.configuration(t))
	if err != nil {
		t.Fatal(err)
	}

	// Called directly: through the SDK, SetProviderAndWait races its own
	// ctx check against the init result.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.InitWithContext(ctx, openfeature.EvaluationContext{}); err != nil {
		t.Fatalf("InitWithContext returned %v, want nil", err)
	}

	result := provider.BooleanEvaluation(context.Background(), "enabled-flag", true, openfeature.FlattenedContext{})
	if code := result.ResolutionDetail().ErrorCode; code != openfeature.FlagNotFoundCode {
		t.Errorf("ErrorCode = %v, want %v", code, openfeature.FlagNotFoundCode)
	}
}

func TestInitFailureLogging(t *testing.T) {
	tests := []struct {
		name        string
		response    fakeResponse
		wantLevel   string
		unwantLevel string
	}{
		{name: "a rejected response logs a warning", response: fakeResponse{status: http.StatusNotFound}, wantLevel: "level=WARN", unwantLevel: "level=ERROR"},
		{name: "a failed request logs an error", response: fakeResponse{body: `{`, contentHash: []byte("hash")}, wantLevel: "level=ERROR", unwantLevel: "level=WARN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeFeatureFlagsServer(t)
			server.setEvaluations(tt.response)
			var logOutput bytes.Buffer
			configuration := server.configuration(t)
			configuration.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))

			startProvider(t, configuration)

			logs := logOutput.String()
			if !strings.Contains(logs, tt.wantLevel) || !strings.Contains(logs, "provider="+providerName) {
				t.Errorf("logs do not contain %q with the provider attribute:\n%s", tt.wantLevel, logs)
			}
			if strings.Contains(logs, tt.unwantLevel) {
				t.Errorf("logs contain %q:\n%s", tt.unwantLevel, logs)
			}
			if strings.Contains(logs, configuration.ClientIdentifier) {
				t.Errorf("logs contain the client identifier:\n%s", logs)
			}
		})
	}
}

func TestNonBooleanEvaluation(t *testing.T) {
	tests := []struct {
		name          string
		slug          string
		wantErrorCode openfeature.ErrorCode
	}{
		{name: "an existing flag is a type mismatch", slug: "enabled-flag", wantErrorCode: openfeature.TypeMismatchCode},
		{name: "a missing flag is not found", slug: "missing-flag", wantErrorCode: openfeature.FlagNotFoundCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeFeatureFlagsServer(t)
			server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
			client := startProvider(t, server.configuration(t))

			details, err := client.StringValueDetails(context.Background(), tt.slug, "default", openfeature.EvaluationContext{})

			if err == nil {
				t.Error("expected an error")
			}
			if details.Value != "default" {
				t.Errorf("Value = %q, want the default", details.Value)
			}
			if details.ErrorCode != tt.wantErrorCode {
				t.Errorf("ErrorCode = %v, want %v", details.ErrorCode, tt.wantErrorCode)
			}
		})
	}
}

type countingTransport struct {
	requests atomic.Int32
}

func (c *countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	c.requests.Add(1)
	return http.DefaultTransport.RoundTrip(request)
}

func TestInjectedHttpClientIsUsed(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
	transport := &countingTransport{}
	configuration := server.configuration(t)
	configuration.HttpClient = &http.Client{Transport: transport}

	client := startProvider(t, configuration)

	if transport.requests.Load() != 1 {
		t.Errorf("injected client made %d requests, want 1", transport.requests.Load())
	}
	if value, _ := client.BooleanValue(context.Background(), "enabled-flag", false, openfeature.EvaluationContext{}); !value {
		t.Error("Value = false, want true")
	}
}

type wrappedTransport struct{ http.RoundTripper }

func TestNewOctopusFeatureProviderToleratesAWrappedDefaultTransport(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = wrappedTransport{original}
	t.Cleanup(func() { http.DefaultTransport = original })

	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
	client := startProvider(t, server.configuration(t))

	if value, _ := client.BooleanValue(context.Background(), "enabled-flag", false, openfeature.EvaluationContext{}); !value {
		t.Error("Value = false, want true")
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
