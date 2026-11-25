package octopus

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OctopusDeploy/openfeature-provider-go/internal/featureflags"
	"github.com/open-feature/go-sdk/openfeature"
	"github.com/open-feature/go-sdk/openfeature/isolated"
)

const testManifest = `[{"slug": "enabled-flag", "value": true, "reason": "The flag is enabled for this environment."}]`

func newTestConfiguration(t *testing.T) *Config {
	t.Helper()

	metadata, err := NewProductMetadata("test-product")
	if err != nil {
		t.Fatal(err)
	}

	return NewConfig("client-identifier", metadata)
}

// startProvider registers a provider for configuration with an isolated SDK
// API, waits for it to initialize, and returns a client bound to it.
func startProvider(t *testing.T, configuration *Config) *openfeature.Client {
	t.Helper()

	provider, err := NewProvider(configuration)
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
	provider, err := NewProvider(server.configuration(t))
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

func TestNewDefaultHttpClientClonesAPlainTransport(t *testing.T) {
	base := &http.Transport{}

	client, owned := newDefaultHttpClient(base)

	if !owned {
		t.Error("owned = false, want true")
	}
	if transport, ok := client.Transport.(*http.Transport); !ok || transport == base {
		t.Errorf("Transport = %v, want a clone of the base transport", client.Transport)
	}
}

func TestNewDefaultHttpClientUsesAWrappedTransportAsIs(t *testing.T) {
	base := wrappedTransport{&http.Transport{}}

	client, owned := newDefaultHttpClient(base)

	if owned {
		t.Error("owned = true, want false")
	}
	if client.Transport != base {
		t.Errorf("Transport = %v, want the wrapped transport", client.Transport)
	}
}

func TestProviderIsUnaffectedByLaterConfigurationChanges(t *testing.T) {
	configuration := newTestConfiguration(t)
	provider, err := NewProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}

	configuration.CacheDuration = time.Hour

	if provider.configuration.CacheDuration != time.Minute {
		t.Errorf("CacheDuration = %v, want the original value", provider.configuration.CacheDuration)
	}
}

func TestNewProviderRejectsNilConfiguration(t *testing.T) {
	if _, err := NewProvider(nil); err == nil {
		t.Error("expected an error for a nil configuration")
	}
}

func TestNewProviderRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		modify   func(*Config)
		wantErrs []string
	}{
		{"empty client identifier", func(c *Config) { c.ClientIdentifier = "" }, []string{"ClientIdentifier"}},
		{"whitespace client identifier", func(c *Config) { c.ClientIdentifier = "  \t" }, []string{"ClientIdentifier"}},
		{"client identifier with a trailing newline", func(c *Config) { c.ClientIdentifier = "client-identifier\n" }, []string{"ClientIdentifier"}},
		{"release version override with a trailing line break", func(c *Config) { c.ReleaseVersionOverride = "1.0\r\n" }, []string{"ReleaseVersionOverride"}},
		{"zero-value product metadata", func(c *Config) { c.ProductMetadata = ProductMetadata{} }, []string{"ProductMetadata"}},
		{"zero cache duration", func(c *Config) { c.CacheDuration = 0 }, []string{"CacheDuration"}},
		{"negative cache duration", func(c *Config) { c.CacheDuration = -time.Second }, []string{"CacheDuration"}},
		{"zero request timeout", func(c *Config) { c.RequestTimeout = 0 }, []string{"RequestTimeout"}},
		{"negative request timeout", func(c *Config) { c.RequestTimeout = -time.Second }, []string{"RequestTimeout"}},
		{
			"every problem at once",
			func(c *Config) { *c = Config{} },
			[]string{"ClientIdentifier", "ProductMetadata", "CacheDuration", "RequestTimeout"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := newTestConfiguration(t)
			test.modify(configuration)

			provider, err := NewProvider(configuration)
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

func TestNewProviderDefaultsServerUriForStructLiteralConfiguration(t *testing.T) {
	metadata, err := NewProductMetadata("test-product")
	if err != nil {
		t.Fatal(err)
	}

	provider, err := NewProvider(&Config{
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

func TestNewProviderAllowsRequestTimeoutLongerThanCacheDuration(t *testing.T) {
	configuration := newTestConfiguration(t)
	configuration.CacheDuration = time.Second
	configuration.RequestTimeout = time.Minute

	if _, err := NewProvider(configuration); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

const testCacheDuration = 10 * time.Millisecond

// syncBuffer collects log output written by the polling goroutine while the
// test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// initProvider initializes a provider without the SDK, so tests can drive
// its lifecycle directly.
func initProvider(t *testing.T, configuration *Config) *Provider {
	t.Helper()

	provider, err := NewProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.InitWithContext(context.Background(), openfeature.EvaluationContext{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.ShutdownWithContext(context.Background()) })

	return provider
}

func evaluate(provider *Provider, slug string) openfeature.BoolResolutionDetail {
	return provider.BooleanEvaluation(context.Background(), slug, false, openfeature.FlattenedContext{})
}

func TestPollingServesAChangedManifest(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: `[{"slug": "enabled-flag", "value": false}]`, contentHash: []byte("old")})
	// Logger stays nil, so this also catches a nil logger reaching the
	// replacement evaluator when it warns about the missing slug.
	configuration := server.configuration(t)
	configuration.CacheDuration = testCacheDuration
	client := startProvider(t, configuration)
	server.waitForRequests(t, checkPath, 1)

	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("new")})
	server.waitForRequests(t, evaluationsPath, 2)
	server.waitForCompletedChecks(t, 0)

	ctx := context.Background()
	if value, _ := client.BooleanValue(ctx, "enabled-flag", false, openfeature.EvaluationContext{}); !value {
		t.Error("Value = false, want the changed manifest's true")
	}
	details, _ := client.BooleanValueDetails(ctx, "missing-flag", false, openfeature.EvaluationContext{})
	if details.ErrorCode != openfeature.FlagNotFoundCode {
		t.Errorf("ErrorCode = %v, want %v", details.ErrorCode, openfeature.FlagNotFoundCode)
	}
}

func TestPollingDoesNotRefetchAnUnchangedManifest(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	configuration := server.configuration(t)
	configuration.CacheDuration = testCacheDuration
	startProvider(t, configuration)

	server.waitForRequests(t, checkPath, 3)

	if count := server.requestCount(evaluationsPath); count != 1 {
		t.Errorf("got %d evaluations requests, want 1", count)
	}
}

func TestPollingFetchesTheEvaluationsDirectlyWhenThereIsNoManifest(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{status: http.StatusInternalServerError})
	configuration := server.configuration(t)
	configuration.CacheDuration = testCacheDuration
	client := startProvider(t, configuration)
	server.waitForRequests(t, evaluationsPath, 2)

	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
	server.waitForRequests(t, checkPath, 1)

	requests := server.recordedRequests()
	for _, request := range requests[:slices.IndexFunc(requests, func(r recordedRequest) bool { return r.path == checkPath })] {
		if request.path != evaluationsPath {
			t.Errorf("request to %s before the first check, want only evaluations requests", request.path)
		}
	}
	if value, _ := client.BooleanValue(context.Background(), "enabled-flag", false, openfeature.EvaluationContext{}); !value {
		t.Error("Value = false, want true")
	}
}

func TestRefreshFailureKeepsTheLastManifest(t *testing.T) {
	tests := []struct {
		name        string
		breakServer func(*fakeFeatureFlagsServer)
	}{
		{"check server error", func(s *fakeFeatureFlagsServer) { s.setCheck(fakeResponse{status: http.StatusInternalServerError}) }},
		{"malformed check body", func(s *fakeFeatureFlagsServer) { s.setCheck(fakeResponse{body: `{`}) }},
		{"missing content hash in check", func(s *fakeFeatureFlagsServer) { s.setCheck(fakeResponse{body: `{}`}) }},
		{"check timeout", func(s *fakeFeatureFlagsServer) { s.setCheck(fakeResponse{delay: time.Minute}) }},
		{"evaluations server error after a change", func(s *fakeFeatureFlagsServer) {
			s.setEvaluations(fakeResponse{status: http.StatusInternalServerError, contentHash: []byte("new")})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeFeatureFlagsServer(t)
			server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("old")})
			var logOutput syncBuffer
			configuration := server.configuration(t)
			configuration.CacheDuration = testCacheDuration
			configuration.RequestTimeout = 50 * time.Millisecond
			configuration.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))
			client := startProvider(t, configuration)

			tt.breakServer(server)
			server.waitForCompletedChecks(t, 1)

			if value, _ := client.BooleanValue(context.Background(), "enabled-flag", false, openfeature.EvaluationContext{}); !value {
				t.Error("Value = false, want the last manifest's true")
			}
			logs := logOutput.String()
			if !strings.Contains(logs, `level=ERROR msg="Failed to retrieve updated feature manifest. Retaining the existing evaluations, which may be stale."`) {
				t.Errorf("logs do not contain the refresh failure:\n%s", logs)
			}
			if strings.Contains(logs, "level=WARN") {
				t.Errorf("logs contain a warning, want only the error:\n%s", logs)
			}
		})
	}
}

func TestShutdownStopsPolling(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
	}{
		{"live context", context.Background()},
		// When replacing a provider, the SDK shuts it down with its original
		// SetProvider ctx, and that ctx may have expired.
		{"cancelled context", cancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newFakeFeatureFlagsServer(t)
			configuration := server.configuration(t)
			configuration.CacheDuration = testCacheDuration
			provider := initProvider(t, configuration)
			server.waitForRequests(t, checkPath, 1)

			_ = provider.ShutdownWithContext(tt.ctx)
			// Lets a poller that's still exiting finish before counting.
			time.Sleep(5 * testCacheDuration)
			count := len(server.recordedRequests())
			time.Sleep(10 * testCacheDuration)

			if got := len(server.recordedRequests()); got != count {
				t.Errorf("got %d requests after shutdown, want none", got-count)
			}
		})
	}
}

func TestShutdownCancelsAnInFlightRequest(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setCheck(fakeResponse{delay: time.Minute})
	configuration := server.configuration(t)
	configuration.CacheDuration = testCacheDuration
	configuration.RequestTimeout = time.Minute
	provider := initProvider(t, configuration)
	server.waitForRequests(t, checkPath, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := provider.ShutdownWithContext(ctx); err != nil {
		t.Errorf("ShutdownWithContext returned %v, want nil", err)
	}
}

func TestSharedProviderFetchesOnceAndShutsDownCleanly(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash")})
	provider, err := NewProvider(server.configuration(t))
	if err != nil {
		t.Fatal(err)
	}

	// The SDK initializes and shuts down the provider once per binding.
	ctx := context.Background()
	api := isolated.NewAPI()
	if err := api.SetProviderAndWait(ctx, provider); err != nil {
		t.Fatal(err)
	}
	if err := api.SetProviderAndWait(ctx, provider, openfeature.WithDomain("other-domain")); err != nil {
		t.Fatal(err)
	}
	if err := api.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown returned %v, want nil", err)
	}

	if count := server.requestCount(evaluationsPath); count != 1 {
		t.Errorf("got %d evaluations requests, want 1", count)
	}
	if !evaluate(provider, "enabled-flag").Value {
		t.Error("Value = false after shutdown, want the last manifest's true")
	}
}

func TestInitAfterShutdownReturnsAnError(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	provider := initProvider(t, server.configuration(t))
	if err := provider.ShutdownWithContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	err := provider.InitWithContext(context.Background(), openfeature.EvaluationContext{})

	if !errors.Is(err, featureflags.ErrShutDown) {
		t.Errorf("InitWithContext returned %v, want %v", err, featureflags.ErrShutDown)
	}
}

func TestInitAfterAnEarlierShutdownStartsNothing(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	configuration := server.configuration(t)
	configuration.CacheDuration = testCacheDuration
	provider, err := NewProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}

	// When the SDK replaces a provider, its shutdown and its pending init
	// run in separate goroutines, so shutdown can arrive first.
	if err := provider.ShutdownWithContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := provider.InitWithContext(context.Background(), openfeature.EvaluationContext{}); err == nil {
		t.Error("InitWithContext returned nil, want an error")
	}

	// A started provider fetches before Init returns, so no requests means
	// nothing started.
	if requests := server.recordedRequests(); len(requests) != 0 {
		t.Errorf("got %d requests, want none: %+v", len(requests), requests)
	}
}

func TestPollingOutlivesTheInitContext(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	configuration := server.configuration(t)
	configuration.CacheDuration = testCacheDuration
	provider, err := NewProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.ShutdownWithContext(context.Background()) })

	ctx, cancel := context.WithCancel(context.Background())
	if err := provider.InitWithContext(ctx, openfeature.EvaluationContext{}); err != nil {
		t.Fatal(err)
	}
	cancel()

	server.waitForRequests(t, checkPath, 2)
}

func TestShutdownCancelsAnInitFetchQuietly(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash"), delay: time.Minute})
	var logOutput syncBuffer
	configuration := server.configuration(t)
	configuration.RequestTimeout = time.Minute
	configuration.Logger = slog.New(slog.NewTextHandler(&logOutput, nil))
	provider, err := NewProvider(configuration)
	if err != nil {
		t.Fatal(err)
	}

	initialized := make(chan error, 1)
	go func() { initialized <- provider.InitWithContext(context.Background(), openfeature.EvaluationContext{}) }()
	server.waitForRequests(t, evaluationsPath, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := provider.ShutdownWithContext(ctx); err != nil {
		t.Errorf("ShutdownWithContext returned %v, want nil", err)
	}
	if err := <-initialized; err != nil {
		t.Errorf("InitWithContext returned %v, want nil", err)
	}

	if logs := logOutput.String(); strings.Contains(logs, "level=ERROR") {
		t.Errorf("logs contain an error:\n%s", logs)
	}
}

func TestSecondInitWaitsForTheFirstFetch(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	server.setEvaluations(fakeResponse{body: testManifest, contentHash: []byte("hash"), delay: 100 * time.Millisecond})
	provider, err := NewProvider(server.configuration(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.ShutdownWithContext(context.Background()) })

	go func() { _ = provider.InitWithContext(context.Background(), openfeature.EvaluationContext{}) }()
	server.waitForRequests(t, evaluationsPath, 1)
	if err := provider.InitWithContext(context.Background(), openfeature.EvaluationContext{}); err != nil {
		t.Fatal(err)
	}

	if !evaluate(provider, "enabled-flag").Value {
		t.Error("Value = false after the second init, want the first fetch's true")
	}
}

type closeTrackingTransport struct {
	http.RoundTripper
	closed atomic.Bool
}

func (c *closeTrackingTransport) CloseIdleConnections() {
	c.closed.Store(true)
}

func TestShutdownLeavesAnInjectedHttpClientOpen(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	transport := &closeTrackingTransport{RoundTripper: http.DefaultTransport}
	configuration := server.configuration(t)
	configuration.HttpClient = &http.Client{Transport: transport}
	provider := initProvider(t, configuration)

	if err := provider.ShutdownWithContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if transport.closed.Load() {
		t.Error("shutdown closed the injected client's idle connections")
	}
}

func TestShutdownClosesAnOwnedHttpClient(t *testing.T) {
	server := newFakeFeatureFlagsServer(t)
	provider, err := NewProvider(server.configuration(t))
	if err != nil {
		t.Fatal(err)
	}
	if provider.ownedHttpClient == nil {
		t.Fatal("ownedHttpClient = nil, want the provider's own client")
	}
	// The API client shares this *http.Client, so its requests go through the
	// tracking transport too.
	transport := &closeTrackingTransport{RoundTripper: provider.ownedHttpClient.Transport}
	provider.ownedHttpClient.Transport = transport
	if err := provider.InitWithContext(context.Background(), openfeature.EvaluationContext{}); err != nil {
		t.Fatal(err)
	}

	if err := provider.ShutdownWithContext(context.Background()); err != nil {
		t.Fatal(err)
	}

	if !transport.closed.Load() {
		t.Error("shutdown didn't close the owned client's idle connections")
	}
}
