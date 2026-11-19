package octopus

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const (
	evaluationsPath = "/api/feature-flags/evaluations/v4/"
	checkPath       = "/api/feature-flags/check/v4/"
)

// fakeResponse is what the fake server returns from one endpoint. The zero
// value is a 200 with an empty body and no ContentHash header.
type fakeResponse struct {
	status      int
	body        string
	contentHash []byte // nil sends no ContentHash header, empty sends an empty one
	delay       time.Duration
}

type recordedRequest struct {
	method string
	path   string
	header http.Header
}

// fakeFeatureFlagsServer stands in for the Octopus feature flags service. By
// default the evaluations endpoint returns an empty manifest, and the check
// endpoint returns the evaluations' content hash.
type fakeFeatureFlagsServer struct {
	*httptest.Server

	mu          sync.Mutex
	evaluations fakeResponse
	requests    []recordedRequest
}

func newFakeFeatureFlagsServer(t *testing.T) *fakeFeatureFlagsServer {
	t.Helper()

	server := &fakeFeatureFlagsServer{
		evaluations: fakeResponse{body: `[]`, contentHash: []byte("initial-hash")},
	}
	server.Server = httptest.NewServer(http.HandlerFunc(server.handle))
	t.Cleanup(server.Close)

	return server
}

// configuration returns a test configuration that points at the server.
func (s *fakeFeatureFlagsServer) configuration(t *testing.T) *OctopusFeatureConfiguration {
	t.Helper()

	configuration := newTestConfiguration(t)
	configuration.serverUri = s.URL

	return configuration
}

func (s *fakeFeatureFlagsServer) setEvaluations(response fakeResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evaluations = response
}

func (s *fakeFeatureFlagsServer) recordedRequests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

func (s *fakeFeatureFlagsServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, recordedRequest{method: r.Method, path: r.URL.Path, header: r.Header.Clone()})

	var response fakeResponse
	switch r.URL.Path {
	case evaluationsPath:
		response = s.evaluations
	case checkPath:
		body, _ := json.Marshal(map[string][]byte{"contentHash": s.evaluations.contentHash})
		response = fakeResponse{body: string(body)}
	default:
		response = fakeResponse{status: http.StatusNotFound}
	}
	s.mu.Unlock()

	if response.delay > 0 {
		select {
		case <-time.After(response.delay):
		case <-r.Context().Done():
			return
		}
	}

	if response.contentHash != nil {
		w.Header().Set("ContentHash", base64.StdEncoding.EncodeToString(response.contentHash))
	}
	if response.status != 0 {
		w.WriteHeader(response.status)
	}
	_, _ = w.Write([]byte(response.body))
}
