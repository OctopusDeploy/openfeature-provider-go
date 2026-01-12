package openfeature

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// octopusClient is the HTTP client for communicating with the Octopus Deploy
// feature toggle service.
type octopusClient struct {
	serverURI        string
	clientIdentifier string
	httpClient       *http.Client
}

// newOctopusClient creates a new Octopus client for fetching feature toggles.
func newOctopusClient(serverURI, clientIdentifier string) *octopusClient {
	return &octopusClient{
		serverURI:        serverURI,
		clientIdentifier: clientIdentifier,
		httpClient:       &http.Client{},
	}
}

// getFeatureToggleEvaluationManifest fetches the complete feature toggle manifest
// from the Octopus Deploy service.
func (c *octopusClient) getFeatureToggleEvaluationManifest() (*FeatureToggles, error) {
	url := fmt.Sprintf("%s/api/featuretoggles/v3/", c.serverURI)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return nil, err
	}

	// Add Bearer token authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.clientIdentifier))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to fetch feature toggles: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		log.Printf("Client configuration not found (404)")
		return nil, fmt.Errorf("client configuration not found")
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Unexpected status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return nil, err
	}

	var evaluations []FeatureToggleEvaluation
	if err := json.Unmarshal(body, &evaluations); err != nil {
		log.Printf("Failed to parse feature toggles: %v", err)
		return nil, err
	}

	// Extract content hash from response header
	contentHashHeader := resp.Header.Get("ContentHash")
	var contentHash []byte
	if contentHashHeader != "" {
		contentHash, err = base64.StdEncoding.DecodeString(contentHashHeader)
		if err != nil {
			log.Printf("Failed to decode content hash: %v", err)
			// Continue without content hash
			contentHash = nil
		}
	}

	return &FeatureToggles{
		Evaluations: evaluations,
		ContentHash: contentHash,
	}, nil
}

// haveFeatureTogglesChanged checks if the feature toggles have changed by comparing
// the content hash with the server.
func (c *octopusClient) haveFeatureTogglesChanged(currentContentHash []byte) (bool, []byte, error) {
	url := fmt.Sprintf("%s/api/featuretoggles/check/v3/", c.serverURI)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create check request: %v", err)
		return false, nil, err
	}

	// Add Bearer token authentication
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.clientIdentifier))

	// Add current content hash if available
	if currentContentHash != nil {
		encodedHash := base64.StdEncoding.EncodeToString(currentContentHash)
		req.Header.Set("ContentHash", encodedHash)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to check for changes: %v", err)
		return false, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Unexpected status code from check endpoint: %d", resp.StatusCode)
		return false, nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Extract new content hash from response header
	newContentHashHeader := resp.Header.Get("ContentHash")
	if newContentHashHeader == "" {
		// If no content hash in response, assume changed
		return true, nil, nil
	}

	newContentHash, err := base64.StdEncoding.DecodeString(newContentHashHeader)
	if err != nil {
		log.Printf("Failed to decode new content hash: %v", err)
		return true, nil, nil
	}

	// Compare hashes
	if currentContentHash == nil {
		return true, newContentHash, nil
	}

	changed := !bytesEqual(currentContentHash, newContentHash)
	return changed, newContentHash, nil
}

// bytesEqual compares two byte slices for equality.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
