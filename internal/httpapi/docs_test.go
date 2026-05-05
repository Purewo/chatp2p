package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAPIDocumentEndpointReturnsCurrentContract(t *testing.T) {
	router := NewRouter(RouterOptions{ServiceName: "chatp2p-test", Version: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected openapi status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/yaml") {
		t.Fatalf("expected yaml content type, got %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "openapi: 3.1.0") {
		t.Fatalf("expected openapi document body, got %q", rec.Body.String())
	}
}

func TestRecentDocsEndpointReturnsMarkdownDocuments(t *testing.T) {
	router := NewRouter(RouterOptions{ServiceName: "chatp2p-test", Version: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/recent?limit=3", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected recent docs status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		Items []struct {
			Path      string `json:"path"`
			Title     string `json:"title"`
			UpdatedAt string `json:"updatedAt"`
			Content   string `json:"content"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode recent docs response: %v", err)
	}
	if len(response.Items) == 0 || len(response.Items) > 3 {
		t.Fatalf("expected one to three docs, got %+v", response.Items)
	}
	for _, item := range response.Items {
		if !strings.HasPrefix(item.Path, "docs/") || !strings.HasSuffix(item.Path, ".md") {
			t.Fatalf("unexpected doc path: %+v", item)
		}
		if item.Title == "" || item.UpdatedAt == "" || item.Content == "" {
			t.Fatalf("expected doc title, updatedAt, and content: %+v", item)
		}
	}
}

func TestRecentDocsEndpointRejectsInvalidLimit(t *testing.T) {
	router := NewRouter(RouterOptions{ServiceName: "chatp2p-test", Version: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs/recent?limit=99", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid limit status %d, got %d: %s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
}
