package httpapi

import (
	"errors"
	"net/http"

	"chatp2p/internal/service"
)

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
	Bio         string `json:"bio"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type updateProfileRequest struct {
	DisplayName *string `json:"displayName"`
	AvatarURL   *string `json:"avatarUrl"`
	Bio         *string `json:"bio"`
}

func (api *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	if api.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "auth service is not configured")
		return
	}

	var req registerRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	session, err := api.auth.Register(r.Context(), service.Credentials{
		Username:    req.Username,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		AvatarURL:   req.AvatarURL,
		Bio:         req.Bio,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, session)
}

func (api *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	if api.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "auth service is not configured")
		return
	}

	var req loginRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	session, err := api.auth.Login(r.Context(), service.Credentials{
		Username: req.Username,
		Password: req.Password,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, session)
}

func (api *API) handleMe(w http.ResponseWriter, r *http.Request) {
	if api.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "auth service is not configured")
		return
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}

	profile, err := api.auth.Authenticate(r.Context(), token)
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (api *API) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	if api.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "auth service is not configured")
		return
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}

	var req updateProfileRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	profile, err := api.auth.UpdateProfile(r.Context(), token, service.ProfileUpdate{
		DisplayName: req.DisplayName,
		AvatarURL:   req.AvatarURL,
		Bio:         req.Bio,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

func (api *API) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_request", "request validation failed")
	case errors.Is(err, service.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials or token")
	case errors.Is(err, service.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "operation is not allowed")
	case errors.Is(err, service.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "resource was not found")
	case errors.Is(err, service.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "resource already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "unexpected server error")
	}
}
