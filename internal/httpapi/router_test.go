package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthEndpointReturnsServiceStatus(t *testing.T) {
	startedAt := time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC)
	router := NewRouter(RouterOptions{
		ServiceName: "chatp2p-test",
		Version:     "test",
		StartedAt:   startedAt,
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var body healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}

	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
	if body.Service != "chatp2p-test" {
		t.Fatalf("expected service chatp2p-test, got %q", body.Service)
	}
	if body.Version != "test" {
		t.Fatalf("expected version test, got %q", body.Version)
	}
	if body.StartedAt != "2026-05-04T01:00:00Z" {
		t.Fatalf("expected startedAt timestamp, got %q", body.StartedAt)
	}
}
