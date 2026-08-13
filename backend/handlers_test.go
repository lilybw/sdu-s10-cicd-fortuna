package main

// ---------------------------------------------------------------------
// Tests for the backend's HTTP handlers (fortuneHandler and healthz).
//
// How to run:
//   cd backend
//   go test -v ./...
//
// These tests call the handlers directly with Go's httptest package —
// httptest.NewRequest builds a fake incoming request, and
// httptest.NewRecorder captures whatever the handler writes back, as
// if it were a real HTTP response. No server actually has to be
// listening on a port for these to run.
//
// testTempDir, resetGlobals, and openBareDB come from database_test.go
// in this same package — they're reused here so we don't duplicate the
// "give me a clean, isolated SQLite file" setup.
// ---------------------------------------------------------------------

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestHandler builds a fortuneHandler wired up to the shared
// datastoreDefault, the same way main() does.
func newTestHandler() *fortuneHandler {
	return &fortuneHandler{store: &datastoreDefault}
}

// seedStore replaces the in-memory fortune map with exactly the given
// fortunes, for tests that don't need the database at all.
func seedStore(t *testing.T, fortunes ...fortune) {
	t.Helper()

	m := map[string]fortune{}
	for _, f := range fortunes {
		m[f.ID] = f
	}

	datastoreDefault.Lock()
	datastoreDefault.m = m
	datastoreDefault.Unlock()
}

// ---------------------------------------------------------------------
// healthz
// ---------------------------------------------------------------------

func TestHealthz_ReturnsOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != "healthy" {
		t.Errorf("got body %q, want %q", rec.Body.String(), "healthy")
	}
}

// ---------------------------------------------------------------------
// fortuneHandler.List  (GET /fortunes)
// ---------------------------------------------------------------------

func TestFortuneHandler_List_ReturnsAllFortunes(t *testing.T) {
	seedStore(t,
		fortune{ID: "1", Message: "first"},
		fortune{ID: "2", Message: "second"},
	)

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/fortunes", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	var got []fortune
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response wasn't valid JSON: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d fortunes, want 2", len(got))
	}
}

func TestFortuneHandler_List_EmptyStoreReturnsEmptyArray(t *testing.T) {
	seedStore(t) // no fortunes

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/fortunes", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	var got []fortune
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response wasn't valid JSON: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d fortunes, want 0", len(got))
	}
}

// ---------------------------------------------------------------------
// fortuneHandler.Get  (GET /fortunes/{id})
// ---------------------------------------------------------------------

func TestFortuneHandler_Get_ReturnsMatchingFortune(t *testing.T) {
	seedStore(t, fortune{ID: "5", Message: "find me"})

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/fortunes/5", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	var got fortune
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response wasn't valid JSON: %v", err)
	}
	if got.Message != "find me" {
		t.Errorf("got message %q, want %q", got.Message, "find me")
	}
}

func TestFortuneHandler_Get_UnknownIDReturnsNotFound(t *testing.T) {
	seedStore(t) // empty

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/fortunes/999", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------
// fortuneHandler.Random  (GET /fortunes/random)
// ---------------------------------------------------------------------

func TestFortuneHandler_Random_ReturnsOneOfTheStoredFortunes(t *testing.T) {
	seedStore(t,
		fortune{ID: "1", Message: "first"},
		fortune{ID: "2", Message: "second"},
		fortune{ID: "3", Message: "third"},
	)

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/fortunes/random", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	var got fortune
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response wasn't valid JSON: %v", err)
	}

	validMessages := map[string]bool{"first": true, "second": true, "third": true}
	if !validMessages[got.Message] {
		t.Errorf("got message %q, which isn't one of the seeded fortunes", got.Message)
	}
}

func TestFortuneHandler_Random_EmptyStoreReturnsNotFound(t *testing.T) {
	seedStore(t) // empty

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/fortunes/random", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// ---------------------------------------------------------------------
// fortuneHandler.Create  (POST /fortunes)
// ---------------------------------------------------------------------

func TestFortuneHandler_Create_SavesToMemoryAndDatabase(t *testing.T) {
	testTempDir(t)
	resetGlobals(t)
	openBareDB(t)
	seedStore(t) // empty

	h := newTestHandler()
	body := strings.NewReader(`{"id": "10", "message": "brand new"}`)
	req := httptest.NewRequest(http.MethodPost, "/fortunes", body)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	// Check it landed in memory.
	datastoreDefault.RLock()
	got, ok := datastoreDefault.m["10"]
	datastoreDefault.RUnlock()
	if !ok {
		t.Fatal("expected fortune 10 to be in the in-memory store")
	}
	if got.Message != "brand new" {
		t.Errorf("got message %q, want %q", got.Message, "brand new")
	}

	// Check it landed in SQLite too.
	var message string
	err := sqliteDB.QueryRow("SELECT message FROM fortunes WHERE id = '10'").Scan(&message)
	if err != nil {
		t.Fatalf("failed to read saved row from database: %v", err)
	}
	if message != "brand new" {
		t.Errorf("got message %q in database, want %q", message, "brand new")
	}
}

// ---------------------------------------------------------------------
// Unknown routes / methods
// ---------------------------------------------------------------------

func TestFortuneHandler_UnsupportedMethodReturnsNotFound(t *testing.T) {
	seedStore(t)

	h := newTestHandler()
	req := httptest.NewRequest(http.MethodDelete, "/fortunes/1", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}
