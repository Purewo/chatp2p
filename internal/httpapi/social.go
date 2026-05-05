package httpapi

import (
	"net/http"
	"strconv"

	"chatp2p/internal/model"
	"chatp2p/internal/service"
)

type userSearchResponse struct {
	Items []model.Profile `json:"items"`
}

type sendFriendRequestRequest struct {
	TargetUserID string `json:"targetUserId"`
	Message      string `json:"message"`
}

type friendRequestListResponse struct {
	Items []model.FriendRequestView `json:"items"`
}

type friendListResponse struct {
	Items []model.Friend `json:"items"`
}

type blockedUserListResponse struct {
	Items []model.BlockedUser `json:"items"`
}

type directConversationRequest struct {
	TargetUserID string `json:"targetUserId"`
}

type groupConversationRequest struct {
	Title     string   `json:"title"`
	MemberIDs []string `json:"memberIds"`
}

func (api *API) handleSearchUsers(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	limit := 20
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a number")
			return
		}
		limit = parsedLimit
	}

	users, err := api.social.SearchUsers(r.Context(), token, r.URL.Query().Get("query"), limit)
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, userSearchResponse{Items: users})
}

func (api *API) handleSendFriendRequest(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	var req sendFriendRequestRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	request, err := api.social.SendFriendRequest(r.Context(), token, service.FriendRequestInput{
		TargetUserID: req.TargetUserID,
		Message:      req.Message,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, request)
}

func (api *API) handleListFriendRequests(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	requests, err := api.social.ListFriendRequests(r.Context(), token, service.FriendRequestListFilter{
		Box:    r.URL.Query().Get("box"),
		Status: r.URL.Query().Get("status"),
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, friendRequestListResponse{Items: requests})
}

func (api *API) handleAcceptFriendRequest(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	result, err := api.social.AcceptFriendRequest(r.Context(), token, r.PathValue("id"))
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (api *API) handleDeclineFriendRequest(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	request, err := api.social.DeclineFriendRequest(r.Context(), token, r.PathValue("id"))
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, request)
}

func (api *API) handleListFriends(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	friends, err := api.social.ListFriends(r.Context(), token)
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, friendListResponse{Items: friends})
}

func (api *API) handleRemoveFriend(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	if err := api.social.RemoveFriend(r.Context(), token, r.PathValue("userId")); err != nil {
		api.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *API) handleListBlockedUsers(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	blocked, err := api.social.ListBlockedUsers(r.Context(), token)
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, blockedUserListResponse{Items: blocked})
}

func (api *API) handleBlockUser(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	blocked, err := api.social.BlockUser(r.Context(), token, r.PathValue("userId"))
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, blocked)
}

func (api *API) handleUnblockUser(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	if err := api.social.UnblockUser(r.Context(), token, r.PathValue("userId")); err != nil {
		api.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *API) handleCreateDirectConversation(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	var req directConversationRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	conversation, err := api.social.GetOrCreateDirectConversation(r.Context(), token, service.DirectConversationInput{
		TargetUserID: req.TargetUserID,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (api *API) handleCreateGroupConversation(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	var req groupConversationRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	conversation, err := api.social.CreateGroupConversation(r.Context(), token, service.GroupConversationInput{
		Title:     req.Title,
		MemberIDs: req.MemberIDs,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, conversation)
}

func (api *API) socialToken(w http.ResponseWriter, r *http.Request) string {
	if api.social == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "social service is not configured")
		return ""
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return ""
	}

	return token
}
