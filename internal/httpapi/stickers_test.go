package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStickerCatalogListsBuiltInClassicPack(t *testing.T) {
	router := NewRouter(RouterOptions{ServiceName: "chatp2p-test", Version: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sticker-packs", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected sticker pack list status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response struct {
		Items []struct {
			ID       string `json:"id"`
			BuiltIn  bool   `json:"builtIn"`
			Stickers []struct {
				ID       string `json:"id"`
				AssetURL string `json:"assetUrl"`
			} `json:"stickers"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode sticker packs: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected one built-in pack, got %+v", response.Items)
	}
	pack := response.Items[0]
	if pack.ID != "classic-faces" || !pack.BuiltIn || len(pack.Stickers) == 0 {
		t.Fatalf("unexpected classic pack: %+v", pack)
	}
	if pack.Stickers[0].AssetURL == "" {
		t.Fatalf("expected sticker asset URL: %+v", pack.Stickers[0])
	}
}

func TestStickerAssetReturnsSVG(t *testing.T) {
	router := NewRouter(RouterOptions{ServiceName: "chatp2p-test", Version: "test"})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stickers/classic-smile/asset.svg", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected sticker asset status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("expected SVG content type, got %q", rec.Header().Get("Content-Type"))
	}
	if !strings.Contains(rec.Body.String(), "<svg") {
		t.Fatalf("expected SVG body, got %q", rec.Body.String())
	}
}
