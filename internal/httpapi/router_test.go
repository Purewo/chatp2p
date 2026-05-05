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

func TestRouterAppliesCORSHeadersForAllowedOrigin(t *testing.T) {
	router := NewRouter(RouterOptions{
		ServiceName:        "chatp2p-test",
		Version:            "test",
		StartedAt:          time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC),
		CORSAllowedOrigins: []string{"http://localhost:5173"},
	})

	preflightReq := httptest.NewRequest(http.MethodOptions, "/api/v1/conversations", nil)
	preflightReq.Header.Set("Origin", "http://localhost:5173")
	preflightReq.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	preflightRec := httptest.NewRecorder()
	router.ServeHTTP(preflightRec, preflightReq)

	if preflightRec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status %d, got %d", http.StatusNoContent, preflightRec.Code)
	}
	if preflightRec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("expected allow origin header, got %q", preflightRec.Header().Get("Access-Control-Allow-Origin"))
	}
	if preflightRec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("expected allow methods header on preflight response")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	getReq.Header.Set("Origin", "http://localhost:5173")
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected health status %d, got %d", http.StatusOK, getRec.Code)
	}
	if getRec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("expected allow origin header on GET, got %q", getRec.Header().Get("Access-Control-Allow-Origin"))
	}
}
