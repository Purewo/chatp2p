package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"chatp2p/internal/ids"
	"chatp2p/internal/model"
	"chatp2p/internal/store"
)

type MessageStore interface {
	FindConversationByID(context.Context, string) (model.ConversationView, error)
	IsConversationMember(context.Context, string, string) (bool, error)
	CreateMessage(context.Context, model.Message) error
	FindMessageByID(context.Context, string) (model.MessageView, error)
	EditMessage(context.Context, string, string, string, string, time.Time) (model.MessageView, error)
	RecallMessage(context.Context, string, string, string, time.Time) (model.MessageView, error)
	ListConversations(context.Context, string, model.ConversationListCursor, time.Time, int, bool) ([]model.ConversationSummary, error)
	UpdateConversationSettings(context.Context, string, string, model.ConversationSettingsUpdate, time.Time) (model.ConversationSettings, error)
	ListMessagesSince(context.Context, string, time.Time, time.Time, int) ([]model.MessageView, error)
	ListMessages(context.Context, string, time.Time, int) ([]model.MessageView, error)
	MarkConversationRead(context.Context, string, string, string, time.Time) (model.ReadThroughResult, error)
}

type MessageService struct {
	auth  *AuthService
	store MessageStore
	now   func() time.Time
}

type MessageInput struct {
	ConversationID string
	Type           string
	Body           string
}

type MessageListFilter struct {
	ConversationID string
	Before         time.Time
	Limit          int
}

type ConversationListFilter struct {
	Before          time.Time
	Cursor          string
	Limit           int
	IncludeArchived bool
}

type ConversationListPage struct {
	Items      []model.ConversationSummary
	NextCursor string
}

type SyncFilter struct {
	Since time.Time
	Limit int
}

type ReadInput struct {
	ConversationID string
	MessageID      string
}

type RecallInput struct {
	ConversationID string
	MessageID      string
}

type EditInput struct {
	ConversationID string
	MessageID      string
	Body           string
}

type ConversationSettingsInput struct {
	ConversationID string
	Pinned         *bool
	MutedUntil     *string
	Archived       *bool
}

type conversationListCursorPayload struct {
	PinnedAt  *time.Time `json:"pinnedAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
	ID        string     `json:"id"`
}

func NewMessageService(authService *AuthService, messageStore MessageStore) *MessageService {
	return &MessageService{
		auth:  authService,
		store: messageStore,
		now:   time.Now,
	}
}

func (s *MessageService) SendMessage(ctx context.Context, token string, input MessageInput) (model.MessageView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.MessageView{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		return model.MessageView{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return model.MessageView{}, err
	}

	messageType := strings.TrimSpace(input.Type)
	if messageType == "" {
		messageType = model.MessageTypeText
	}
	if messageType != model.MessageTypeText {
		return model.MessageView{}, ErrInvalidInput
	}

	body := strings.TrimSpace(input.Body)
	if body == "" || utf8.RuneCountInString(body) > 4000 {
		return model.MessageView{}, ErrInvalidInput
	}

	messageID, err := ids.New()
	if err != nil {
		return model.MessageView{}, err
	}

	now := s.now().UTC()
	message := model.Message{
		ID:             messageID,
		ConversationID: conversationID,
		SenderID:       actor.ID,
		Type:           messageType,
		Body:           body,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.store.CreateMessage(ctx, message); err != nil {
		return model.MessageView{}, mapMessageStoreError(err)
	}

	return s.store.FindMessageByID(ctx, messageID)
}

func (s *MessageService) ListMessages(ctx context.Context, token string, filter MessageListFilter) ([]model.MessageView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	conversationID := strings.TrimSpace(filter.ConversationID)
	if conversationID == "" {
		return nil, ErrInvalidInput
	}
	if filter.Limit < 0 || filter.Limit > 100 {
		return nil, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return nil, err
	}

	return s.store.ListMessages(ctx, conversationID, filter.Before, filter.Limit)
}

func (s *MessageService) ListConversations(ctx context.Context, token string, filter ConversationListFilter) (ConversationListPage, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return ConversationListPage{}, err
	}

	if filter.Limit < 0 || filter.Limit > 100 || (filter.Cursor != "" && !filter.Before.IsZero()) {
		return ConversationListPage{}, ErrInvalidInput
	}

	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	cursor, err := decodeConversationListCursor(filter.Cursor)
	if err != nil {
		return ConversationListPage{}, ErrInvalidInput
	}

	conversations, err := s.store.ListConversations(ctx, actor.ID, cursor, filter.Before, limit+1, filter.IncludeArchived)
	if err != nil {
		return ConversationListPage{}, err
	}

	page := ConversationListPage{Items: conversations}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		nextCursor, err := encodeConversationListCursor(page.Items[len(page.Items)-1])
		if err != nil {
			return ConversationListPage{}, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
}

func decodeConversationListCursor(raw string) (model.ConversationListCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.ConversationListCursor{}, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return model.ConversationListCursor{}, err
	}

	var payload conversationListCursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.ConversationListCursor{}, err
	}
	payload.ID = strings.TrimSpace(payload.ID)
	if payload.ID == "" || payload.UpdatedAt.IsZero() {
		return model.ConversationListCursor{}, ErrInvalidInput
	}

	cursor := model.ConversationListCursor{
		Valid:     true,
		PinnedAt:  payload.PinnedAt,
		UpdatedAt: payload.UpdatedAt.UTC(),
		ID:        payload.ID,
	}
	if cursor.PinnedAt != nil {
		pinnedAt := cursor.PinnedAt.UTC()
		cursor.PinnedAt = &pinnedAt
	}
	return cursor, nil
}

func encodeConversationListCursor(conversation model.ConversationSummary) (string, error) {
	payload := conversationListCursorPayload{
		PinnedAt:  conversation.PinnedAt,
		UpdatedAt: conversation.UpdatedAt.UTC(),
		ID:        conversation.ID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func (s *MessageService) Sync(ctx context.Context, token string, filter SyncFilter) (model.SyncSnapshot, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.SyncSnapshot{}, err
	}

	if filter.Since.IsZero() || filter.Limit < 0 || filter.Limit > 100 {
		return model.SyncSnapshot{}, ErrInvalidInput
	}

	serverTime := s.now().UTC()
	conversations, err := s.store.ListConversations(ctx, actor.ID, model.ConversationListCursor{}, time.Time{}, filter.Limit, true)
	if err != nil {
		return model.SyncSnapshot{}, err
	}

	messages, err := s.store.ListMessagesSince(ctx, actor.ID, filter.Since, serverTime, filter.Limit)
	if err != nil {
		return model.SyncSnapshot{}, err
	}

	return model.SyncSnapshot{
		ServerTime:    serverTime,
		Conversations: conversations,
		Messages:      messages,
	}, nil
}

func (s *MessageService) ConversationForUser(ctx context.Context, userID, conversationID string) (model.ConversationView, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return model.ConversationView{}, ErrInvalidInput
	}
	if err := s.requireConversationMemberForUser(ctx, conversationID, userID); err != nil {
		return model.ConversationView{}, err
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *MessageService) MarkRead(ctx context.Context, token string, input ReadInput) (model.ReadThroughResult, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ReadThroughResult{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	messageID := strings.TrimSpace(input.MessageID)
	if conversationID == "" || messageID == "" {
		return model.ReadThroughResult{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return model.ReadThroughResult{}, err
	}

	result, err := s.store.MarkConversationRead(ctx, conversationID, actor.ID, messageID, s.now().UTC())
	if err != nil {
		return model.ReadThroughResult{}, mapMessageStoreError(err)
	}
	return result, nil
}

func (s *MessageService) UpdateConversationSettings(ctx context.Context, token string, input ConversationSettingsInput) (model.ConversationSettings, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationSettings{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		return model.ConversationSettings{}, ErrInvalidInput
	}
	if input.Pinned == nil && input.MutedUntil == nil && input.Archived == nil {
		return model.ConversationSettings{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return model.ConversationSettings{}, err
	}

	now := s.now().UTC()
	update := model.ConversationSettingsUpdate{
		Pinned:   input.Pinned,
		Archived: input.Archived,
	}
	if input.MutedUntil != nil {
		update.UpdateMutedUntil = true
		rawMutedUntil := strings.TrimSpace(*input.MutedUntil)
		if rawMutedUntil != "" {
			mutedUntil, err := time.Parse(time.RFC3339, rawMutedUntil)
			if err != nil || !mutedUntil.After(now) {
				return model.ConversationSettings{}, ErrInvalidInput
			}
			mutedUntil = mutedUntil.UTC()
			update.MutedUntil = &mutedUntil
		}
	}

	settings, err := s.store.UpdateConversationSettings(ctx, conversationID, actor.ID, update, now)
	if err != nil {
		return model.ConversationSettings{}, mapMessageStoreError(err)
	}
	return settings, nil
}

func (s *MessageService) EditMessage(ctx context.Context, token string, input EditInput) (model.MessageView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.MessageView{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	messageID := strings.TrimSpace(input.MessageID)
	body := strings.TrimSpace(input.Body)
	if conversationID == "" || messageID == "" || body == "" || utf8.RuneCountInString(body) > 4000 {
		return model.MessageView{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return model.MessageView{}, err
	}

	message, err := s.store.EditMessage(ctx, conversationID, messageID, actor.ID, body, s.now().UTC())
	if err != nil {
		return model.MessageView{}, mapMessageStoreError(err)
	}
	return message, nil
}

func (s *MessageService) RecallMessage(ctx context.Context, token string, input RecallInput) (model.MessageView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.MessageView{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	messageID := strings.TrimSpace(input.MessageID)
	if conversationID == "" || messageID == "" {
		return model.MessageView{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return model.MessageView{}, err
	}

	message, err := s.store.RecallMessage(ctx, conversationID, messageID, actor.ID, s.now().UTC())
	if err != nil {
		return model.MessageView{}, mapMessageStoreError(err)
	}
	return message, nil
}

func (s *MessageService) Conversation(ctx context.Context, token, conversationID string) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return model.ConversationView{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return model.ConversationView{}, err
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *MessageService) requireConversationMember(ctx context.Context, conversationID, userID string) error {
	return s.requireConversationMemberForUser(ctx, conversationID, userID)
}

func (s *MessageService) requireConversationMemberForUser(ctx context.Context, conversationID, userID string) error {
	if _, err := s.store.FindConversationByID(ctx, conversationID); err != nil {
		return mapMessageStoreError(err)
	}

	isMember, err := s.store.IsConversationMember(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return ErrForbidden
	}
	return nil
}

func mapMessageStoreError(err error) error {
	switch {
	case errors.Is(err, store.ErrConversationNotFound), errors.Is(err, store.ErrConversationMemberNotFound), errors.Is(err, store.ErrMessageNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, store.ErrMessageAlreadyRecalled):
		return ErrConflict
	default:
		return err
	}
}
