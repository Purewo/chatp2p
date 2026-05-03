package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"chatp2p/internal/model"
	"chatp2p/internal/service"
)

type sendMessageRequest struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

type editMessageRequest struct {
	Body string `json:"body"`
}

type messageListResponse struct {
	Items []model.MessageView `json:"items"`
}

type conversationListResponse struct {
	Items []model.ConversationSummary `json:"items"`
}

type markReadRequest struct {
	MessageID string `json:"messageId"`
}

func (api *API) handleSync(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	since, err := parseRequiredTimeQuery(r.URL.Query().Get("since"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "since must be an RFC3339 timestamp")
		return
	}

	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = parsePositiveInt(rawLimit, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
	}

	snapshot, err := api.messages.Sync(r.Context(), token, service.SyncFilter{
		Since: since,
		Limit: limit,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, snapshot)
}

func (api *API) handleListConversations(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	before, err := parseTimeQuery(r.URL.Query().Get("before"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "before must be an RFC3339 timestamp")
		return
	}

	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = parsePositiveInt(rawLimit, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
	}

	conversations, err := api.messages.ListConversations(r.Context(), token, service.ConversationListFilter{
		Before: before,
		Limit:  limit,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, conversationListResponse{Items: conversations})
}

func (api *API) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req sendMessageRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	message, err := api.messages.SendMessage(r.Context(), token, service.MessageInput{
		ConversationID: r.PathValue("conversationId"),
		Type:           req.Type,
		Body:           req.Body,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if conversation, err := api.messages.Conversation(r.Context(), token, message.ConversationID); err == nil {
		api.publishConversationEvent(conversation, eventMessageCreated, message)
	}

	writeJSON(w, http.StatusCreated, message)
}

func (api *API) handleListMessages(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	before, err := parseTimeQuery(r.URL.Query().Get("before"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "before must be an RFC3339 timestamp")
		return
	}

	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		limit, err = parsePositiveInt(rawLimit, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
	}

	messages, err := api.messages.ListMessages(r.Context(), token, service.MessageListFilter{
		ConversationID: r.PathValue("conversationId"),
		Before:         before,
		Limit:          limit,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, messageListResponse{Items: messages})
}

func (api *API) handleEditMessage(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req editMessageRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	message, err := api.messages.EditMessage(r.Context(), token, service.EditInput{
		ConversationID: r.PathValue("conversationId"),
		MessageID:      r.PathValue("messageId"),
		Body:           req.Body,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if conversation, err := api.messages.Conversation(r.Context(), token, message.ConversationID); err == nil {
		api.publishConversationEvent(conversation, eventMessageEdited, message)
	}

	writeJSON(w, http.StatusOK, message)
}

func (api *API) handleRecallMessage(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	message, err := api.messages.RecallMessage(r.Context(), token, service.RecallInput{
		ConversationID: r.PathValue("conversationId"),
		MessageID:      r.PathValue("messageId"),
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if conversation, err := api.messages.Conversation(r.Context(), token, message.ConversationID); err == nil {
		api.publishConversationEvent(conversation, eventMessageRecalled, message)
	}

	writeJSON(w, http.StatusOK, message)
}

func (api *API) handleMarkConversationRead(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req markReadRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	result, err := api.messages.MarkRead(r.Context(), token, service.ReadInput{
		ConversationID: r.PathValue("conversationId"),
		MessageID:      req.MessageID,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if profile, profileErr := api.auth.Authenticate(r.Context(), token); profileErr == nil {
		if conversation, conversationErr := api.messages.Conversation(r.Context(), token, result.ConversationID); conversationErr == nil {
			api.publishConversationEvent(conversation, eventMessageRead, model.MessageReadEvent{
				ConversationID:       result.ConversationID,
				ReadThroughMessageID: result.ReadThroughMessageID,
				Reader:               profile,
				ReadAt:               result.ReadAt,
			})
		}
	}

	writeJSON(w, http.StatusOK, result)
}

func (api *API) requireToken(w http.ResponseWriter, r *http.Request, configured bool) string {
	if !configured {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "message service is not configured")
		return ""
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return ""
	}
	return token
}

func parseTimeQuery(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

func parseRequiredTimeQuery(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, strconv.ErrSyntax
	}
	return time.Parse(time.RFC3339, value)
}

func parsePositiveInt(raw string, max int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > max {
		if err == nil {
			err = strconv.ErrSyntax
		}
		return 0, err
	}
	return n, nil
}
