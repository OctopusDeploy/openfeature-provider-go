package featureflags

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// FeatureFlagApiClientOptions configures a [FeatureFlagApiClient].
type FeatureFlagApiClientOptions struct {
	ServerUri              string
	ClientIdentifier       string
	ProductName            string
	ProductVersion         string // sent only when not empty
	ProviderVersion        string
	ReleaseVersionOverride string // sent only when not empty
	RequestTimeout         time.Duration
	HttpClient             *http.Client
}

// FeatureFlagApiClient retrieves feature flag evaluations from the Octopus
// feature flags service.
type FeatureFlagApiClient struct {
	options FeatureFlagApiClientOptions
}

// NewFeatureFlagApiClient returns a client for options. HttpClient must not
// be nil.
func NewFeatureFlagApiClient(options FeatureFlagApiClientOptions) *FeatureFlagApiClient {
	return &FeatureFlagApiClient{options: options}
}

// RejectedResponseError is returned when the service responds, but not with
// a usable manifest. The cache logs it as a warning during init, and as an
// error when a refresh fails. Other failures are always errors.
type RejectedResponseError struct {
	Message string
	Attrs   []slog.Attr
}

func (e *RejectedResponseError) Error() string {
	return e.Message
}

// HaveFeaturesChanged asks the service whether the evaluations differ from
// the ones with contentHash. An empty contentHash means nothing has been
// fetched yet, so it reports a change without asking. The request is bounded
// by RequestTimeout as well as ctx.
func (c *FeatureFlagApiClient) HaveFeaturesChanged(ctx context.Context, contentHash []byte) (bool, error) {
	if len(contentHash) == 0 {
		return true, nil
	}

	ctx, cancel := context.WithTimeout(ctx, c.options.RequestTimeout)
	defer cancel()

	response, err := c.get(ctx, "api/feature-flags/check/v4/")
	if err != nil {
		return false, err
	}
	defer closeResponse(response)

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return false, rejected("Feature flag check returned an unsuccessful status code", slog.String("serverUri", c.options.ServerUri), slog.Int("status", response.StatusCode))
	}

	var check struct {
		ContentHash []byte `json:"contentHash"`
	}
	if err := json.NewDecoder(response.Body).Decode(&check); err != nil {
		return false, fmt.Errorf("decoding the check response: %w", err)
	}
	if check.ContentHash == nil {
		return false, rejected("Failed to retrieve feature flags for client identifier. Check did not return a valid content hash.", slog.String("serverUri", c.options.ServerUri))
	}

	return !bytes.Equal(check.ContentHash, contentHash), nil
}

// GetServerSideEvaluations fetches the current v4 evaluations. Each request
// is bounded by RequestTimeout as well as ctx.
func (c *FeatureFlagApiClient) GetServerSideEvaluations(ctx context.Context) (EvaluationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.options.RequestTimeout)
	defer cancel()

	response, err := c.get(ctx, "api/feature-flags/evaluations/v4/")
	if err != nil {
		return EvaluationResponse{}, err
	}
	defer closeResponse(response)

	serverUri := slog.String("serverUri", c.options.ServerUri)

	if response.StatusCode == http.StatusNotFound {
		return EvaluationResponse{}, rejected("Failed to retrieve feature flags for client identifier", serverUri)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return EvaluationResponse{}, rejected("Feature flag request returned an unsuccessful status code", serverUri, slog.Int("status", response.StatusCode))
	}

	contentHashes := response.Header.Values("ContentHash")
	if len(contentHashes) == 0 {
		return EvaluationResponse{}, rejected("Feature flag response did not contain expected ContentHash header", serverUri)
	}
	if contentHashes[0] == "" {
		return EvaluationResponse{}, rejected("Feature flag response returned an empty ContentHash header", serverUri)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return EvaluationResponse{}, fmt.Errorf("reading the evaluations response: %w", err)
	}

	evaluations, err := ParseEvaluations(body)
	if errors.Is(err, ErrNullEvaluations) {
		return EvaluationResponse{}, rejected("Feature flag response content was empty", serverUri)
	}
	if err != nil {
		return EvaluationResponse{}, err
	}

	contentHash, err := base64.StdEncoding.DecodeString(contentHashes[0])
	if err != nil {
		return EvaluationResponse{}, fmt.Errorf("decoding the ContentHash header: %w", err)
	}

	return EvaluationResponse{Evaluations: evaluations, ContentHash: contentHash}, nil
}

func (c *FeatureFlagApiClient) get(ctx context.Context, path string) (*http.Response, error) {
	requestUrl, err := url.JoinPath(c.options.ServerUri, path)
	if err != nil {
		return nil, fmt.Errorf("building the request URL: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("building the request: %w", err)
	}

	request.Header.Set("Authorization", "Bearer "+c.options.ClientIdentifier)
	request.Header.Set("X-Octopus-Client", c.octopusClientHeader())
	if c.options.ReleaseVersionOverride != "" {
		request.Header.Set("X-Release-Version", c.options.ReleaseVersionOverride)
	}

	return c.options.HttpClient.Do(request)
}

func (c *FeatureFlagApiClient) octopusClientHeader() string {
	product := c.options.ProductName
	if c.options.ProductVersion != "" {
		product += "/" + c.options.ProductVersion
	}

	return product + " openfeature-provider-go/" + c.options.ProviderVersion
}

// closeResponse drains what's left of the body so the connection can be
// reused.
func closeResponse(response *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	_ = response.Body.Close()
}

func rejected(message string, attrs ...slog.Attr) error {
	return &RejectedResponseError{Message: message, Attrs: attrs}
}
