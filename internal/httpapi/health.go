package httpapi

import (
	"net/http"
	"time"
)

type healthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	StartedAt string `json:"startedAt"`
}

func (api *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{
		Status:    "ok",
		Service:   api.serviceName,
		Version:   api.version,
		StartedAt: api.startedAt.Format(time.RFC3339),
	})
}
