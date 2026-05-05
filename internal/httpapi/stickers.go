package httpapi

import (
	"net/http"

	"chatp2p/internal/model"
)

type stickerPackListResponse struct {
	Items []model.StickerPack `json:"items"`
}

type stickerListResponse struct {
	Items []model.Sticker `json:"items"`
}

func (api *API) handleListStickerPacks(w http.ResponseWriter, r *http.Request) {
	if api.stickers == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "sticker service is not configured")
		return
	}

	writeJSON(w, http.StatusOK, stickerPackListResponse{Items: api.stickers.ListPacks()})
}

func (api *API) handleListStickers(w http.ResponseWriter, r *http.Request) {
	if api.stickers == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "sticker service is not configured")
		return
	}

	stickers, ok := api.stickers.ListStickers(r.PathValue("packId"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "sticker pack was not found")
		return
	}

	writeJSON(w, http.StatusOK, stickerListResponse{Items: stickers})
}

func (api *API) handleStickerAsset(w http.ResponseWriter, r *http.Request) {
	if api.stickers == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "sticker service is not configured")
		return
	}

	asset, ok := api.stickers.Asset(r.PathValue("stickerId"))
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "sticker was not found")
		return
	}

	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(asset.Body)
}
