package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"chatp2p/internal/model"
	"chatp2p/internal/service"
)

type sendMessageRequest struct {
	Type           string `json:"type"`
	Body           string `json:"body"`
	QuoteMessageID string `json:"quoteMessageId"`
}

type editMessageRequest struct {
	Body string `json:"body"`
}

type forwardMessageRequest struct {
	TargetConversationID string `json:"targetConversationId"`
}

type forwardMessagesRequest struct {
	MessageIDs           []string `json:"messageIds"`
	TargetConversationID string   `json:"targetConversationId"`
}

type messageIDsRequest struct {
	MessageIDs []string `json:"messageIds"`
}

type conversationSettingsRequest struct {
	Pinned     *bool   `json:"pinned"`
	MutedUntil *string `json:"mutedUntil"`
	Archived   *bool   `json:"archived"`
}

type messageListResponse struct {
	Items      []model.MessageView `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

type conversationListResponse struct {
	Items      []model.ConversationSummary `json:"items"`
	NextCursor string                      `json:"nextCursor,omitempty"`
}

type markReadRequest struct {
	MessageID string `json:"messageId"`
}

type messageBatchResponse struct {
	Items []model.MessageView `json:"items"`
}

type messageFavoriteListResponse struct {
	Items []model.MessageFavorite `json:"items"`
}

type messageDeleteBatchResponse struct {
	DeletedMessageIDs []string `json:"deletedMessageIds"`
}

func (api *API) handleSync(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	since, err := parseTimeQuery(r.URL.Query().Get("since"))
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
		Since:  since,
		Cursor: r.URL.Query().Get("cursor"),
		Limit:  limit,
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

	includeArchived := false
	if rawIncludeArchived := r.URL.Query().Get("includeArchived"); rawIncludeArchived != "" {
		includeArchived, err = strconv.ParseBool(rawIncludeArchived)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "includeArchived must be a boolean")
			return
		}
	}

	page, err := api.messages.ListConversations(r.Context(), token, service.ConversationListFilter{
		Before:          before,
		Cursor:          r.URL.Query().Get("cursor"),
		Limit:           limit,
		IncludeArchived: includeArchived,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, conversationListResponse{
		Items:      page.Items,
		NextCursor: page.NextCursor,
	})
}

func (api *API) handleUpdateConversationSettings(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req conversationSettingsRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	settings, err := api.messages.UpdateConversationSettings(r.Context(), token, service.ConversationSettingsInput{
		ConversationID: r.PathValue("conversationId"),
		Pinned:         req.Pinned,
		MutedUntil:     req.MutedUntil,
		Archived:       req.Archived,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, settings)
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
		QuoteMessageID: req.QuoteMessageID,
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

	page, err := api.messages.ListMessages(r.Context(), token, service.MessageListFilter{
		ConversationID: r.PathValue("conversationId"),
		Before:         before,
		Cursor:         r.URL.Query().Get("cursor"),
		Limit:          limit,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, messageListResponse{
		Items:      page.Items,
		NextCursor: page.NextCursor,
	})
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

func (api *API) handleForwardMessage(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req forwardMessageRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	messages, err := api.messages.ForwardMessages(r.Context(), token, service.ForwardInput{
		SourceConversationID: r.PathValue("conversationId"),
		MessageIDs:           []string{r.PathValue("messageId")},
		TargetConversationID: req.TargetConversationID,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	api.publishCreatedMessages(r, token, messages)
	writeJSON(w, http.StatusCreated, messages[0])
}

func (api *API) handleForwardMessages(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req forwardMessagesRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	messages, err := api.messages.ForwardMessages(r.Context(), token, service.ForwardInput{
		SourceConversationID: r.PathValue("conversationId"),
		MessageIDs:           req.MessageIDs,
		TargetConversationID: req.TargetConversationID,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	api.publishCreatedMessages(r, token, messages)
	writeJSON(w, http.StatusCreated, messageBatchResponse{Items: messages})
}

func (api *API) handleDeleteMessageForMe(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	if _, err := api.messages.DeleteMessagesForMe(r.Context(), token, service.MessageIDsInput{
		ConversationID: r.PathValue("conversationId"),
		MessageIDs:     []string{r.PathValue("messageId")},
	}); err != nil {
		api.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *API) handleDeleteMessagesForMe(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req messageIDsRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	deletedIDs, err := api.messages.DeleteMessagesForMe(r.Context(), token, service.MessageIDsInput{
		ConversationID: r.PathValue("conversationId"),
		MessageIDs:     req.MessageIDs,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, messageDeleteBatchResponse{DeletedMessageIDs: deletedIDs})
}

func (api *API) handleFavoriteMessage(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	favorites, err := api.messages.FavoriteMessages(r.Context(), token, service.MessageIDsInput{
		ConversationID: r.PathValue("conversationId"),
		MessageIDs:     []string{r.PathValue("messageId")},
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, favorites[0])
}

func (api *API) handleFavoriteMessages(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	var req messageIDsRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	favorites, err := api.messages.FavoriteMessages(r.Context(), token, service.MessageIDsInput{
		ConversationID: r.PathValue("conversationId"),
		MessageIDs:     req.MessageIDs,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, messageFavoriteListResponse{Items: favorites})
}

func (api *API) handleUnfavoriteMessage(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	if err := api.messages.UnfavoriteMessage(r.Context(), token, service.MessageIDsInput{
		ConversationID: r.PathValue("conversationId"),
		MessageIDs:     []string{r.PathValue("messageId")},
	}); err != nil {
		api.writeServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *API) handleListMessageFavorites(w http.ResponseWriter, r *http.Request) {
	token := api.requireToken(w, r, api.messages != nil)
	if token == "" {
		return
	}

	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		var err error
		limit, err = parsePositiveInt(rawLimit, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
			return
		}
	}

	favorites, err := api.messages.ListMessageFavorites(r.Context(), token, service.FavoriteListFilter{Limit: limit})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, messageFavoriteListResponse{Items: favorites})
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

func (api *API) publishCreatedMessages(r *http.Request, token string, messages []model.MessageView) {
	if len(messages) == 0 {
		return
	}
	if conversation, err := api.messages.Conversation(r.Context(), token, messages[0].ConversationID); err == nil {
		for _, message := range messages {
			api.publishConversationEvent(conversation, eventMessageCreated, message)
		}
	}
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
