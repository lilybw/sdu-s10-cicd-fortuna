package main

// ---------------------------------------------------------------------
// Tests for the frontend's API handlers.
//
// How to run:
//   cd frontend
//   go test -v ./...
//
// The frontend doesn't have a database of its own — its whole job is
// to call the backend over HTTP and reshape the response. So instead
// of a real backend, we spin up a fake one with httptest.NewServer()
// that returns exactly the response we want for each test, and point
// backendBaseURL at it. This is what "mocking the endpoint" means
// here: nothing here needs the real backend running.
// ---------------------------------------------------------------------

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withMockBackend spins up a fake backend server that runs the given
// handler, points backendBaseURL at it for the duration of the test,
// and cleans everything up (closes the server, restores the original
// URL) automatically when the test finishes.
func withMockBackend(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(handler)

	original := backendBaseURL
	backendBaseURL = server.URL

	t.Cleanup(func() {
		server.Close()
		backendBaseURL = original
	})

	return server
}

// ---------------------------------------------------------------------
// HealthzHandler
// ---------------------------------------------------------------------

func TestHealthzHandler_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	HealthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "healthy" {
		t.Errorf("got body %q, want %q", rec.Body.String(), "healthy")
	}
}

// ---------------------------------------------------------------------
// RandomHandler
// ---------------------------------------------------------------------

func TestRandomHandler_ReturnsFortuneMessageFromBackend(t *testing.T) {
	// Fake backend: whatever it's asked, it always returns this one fortune.
	withMockBackend(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fortunes/random" {
			t.Errorf("frontend called unexpected path %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(fortune{ID: "1", Message: "mocked fortune"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/random", nil)
	rec := httptest.NewRecorder()

	RandomHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "mocked fortune" {
		t.Errorf("got body %q, want %q", rec.Body.String(), "mocked fortune")
	}
}

func TestRandomHandler_BackendDownReturnsBadGateway(t *testing.T) {
	// Point at a mock server, then immediately close it, so any request
	// to it fails the way a real "backend is down" failure would.
	server := withMockBackend(t, func(w http.ResponseWriter, r *http.Request) {})
	server.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/random", nil)
	rec := httptest.NewRecorder()

	RandomHandler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

// ---------------------------------------------------------------------
// AllHandler
// ---------------------------------------------------------------------

func TestAllHandler_RendersFortunesFromBackend(t *testing.T) {
	withMockBackend(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/fortunes" {
			t.Errorf("frontend called unexpected path %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]fortune{
			{ID: "1", Message: "first fortune"},
			{ID: "2", Message: "second fortune"},
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/all", nil)
	rec := httptest.NewRecorder()

	AllHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "1: first fortune") {
		t.Errorf("expected body to contain %q, got: %s", "1: first fortune", body)
	}
	if !strings.Contains(body, "2: second fortune") {
		t.Errorf("expected body to contain %q, got: %s", "2: second fortune", body)
	}
}

func TestAllHandler_BackendDownReturnsBadGateway(t *testing.T) {
	server := withMockBackend(t, func(w http.ResponseWriter, r *http.Request) {})
	server.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/all", nil)
	rec := httptest.NewRecorder()

	AllHandler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

// ---------------------------------------------------------------------
// AddHandler
// ---------------------------------------------------------------------

func TestAddHandler_RejectsNonPostRequests(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/add", nil)
	rec := httptest.NewRecorder()

	AddHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestAddHandler_ForwardsMessageToBackend(t *testing.T) {
	var receivedBody string

	withMockBackend(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("frontend used method %q, want POST", r.Method)
		}
		if r.URL.Path != "/fortunes" {
			t.Errorf("frontend called unexpected path %q", r.URL.Path)
		}

		buf := make([]byte, r.ContentLength)
		r.Body.Read(buf)
		receivedBody = string(buf)

		w.WriteHeader(http.StatusOK)
	})

	reqBody := strings.NewReader(`{"message": "a brand new fortune"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/add", reqBody)
	rec := httptest.NewRecorder()

	AddHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "Cookie added!" {
		t.Errorf("got body %q, want %q", rec.Body.String(), "Cookie added!")
	}
	if !strings.Contains(receivedBody, "a brand new fortune") {
		t.Errorf("backend did not receive the fortune message, got: %s", receivedBody)
	}
}

func TestAddHandler_BackendDownReturnsBadGateway(t *testing.T) {
	server := withMockBackend(t, func(w http.ResponseWriter, r *http.Request) {})
	server.Close()

	reqBody := strings.NewReader(`{"message": "doesn't matter"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/add", reqBody)
	rec := httptest.NewRecorder()

	AddHandler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusBadGateway)
	}
}
