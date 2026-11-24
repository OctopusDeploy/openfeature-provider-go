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
	check       *fakeResponse // nil serves the evaluations' content hash
	requests    []recordedRequest
	requested   chan struct{} // closed and replaced on every request
}

func newFakeFeatureFlagsServer(t *testing.T) *fakeFeatureFlagsServer {
	t.Helper()

	server := &fakeFeatureFlagsServer{
		evaluations: fakeResponse{body: `[]`, contentHash: []byte("initial-hash")},
		requested:   make(chan struct{}),
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

// setCheck overrides the check endpoint's response.
func (s *fakeFeatureFlagsServer) setCheck(response fakeResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.check = &response
}

// requestCount returns how many requests have been made to path.
func (s *fakeFeatureFlagsServer) requestCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestCountLocked(path)
}

func (s *fakeFeatureFlagsServer) requestCountLocked(path string) int {
	count := 0
	for _, request := range s.requests {
		if request.path == path {
			count++
		}
	}
	return count
}

// waitForRequests waits until at least n requests have been made to path.
func (s *fakeFeatureFlagsServer) waitForRequests(t *testing.T, path string, n int) {
	t.Helper()

	timeout := time.After(5 * time.Second)
	for {
		s.mu.Lock()
		count, requested := s.requestCountLocked(path), s.requested
		s.mu.Unlock()

		if count >= n {
			return
		}

		select {
		case <-requested:
		case <-timeout:
			t.Fatalf("got %d requests to %s, want at least %d", count, path, n)
		}
	}
}

// waitForCompletedChecks waits until n more checks have completed. The
// provider checks one at a time, so once the next check starts, the previous
// one and anything it triggered have finished.
func (s *fakeFeatureFlagsServer) waitForCompletedChecks(t *testing.T, n int) {
	t.Helper()

	s.waitForRequests(t, checkPath, s.requestCount(checkPath)+n+1)
}

func (s *fakeFeatureFlagsServer) recordedRequests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

func (s *fakeFeatureFlagsServer) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, recordedRequest{method: r.Method, path: r.URL.Path, header: r.Header.Clone()})
	close(s.requested)
	s.requested = make(chan struct{})

	var response fakeResponse
	switch r.URL.Path {
	case evaluationsPath:
		response = s.evaluations
	case checkPath:
		if s.check != nil {
			response = *s.check
		} else {
			body, _ := json.Marshal(map[string][]byte{"contentHash": s.evaluations.contentHash})
			response = fakeResponse{body: string(body)}
		}
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
