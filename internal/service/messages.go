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
	FindVisibleMessageByID(context.Context, string, string, string) (model.MessageView, error)
	EditMessage(context.Context, string, string, string, string, time.Time) (model.MessageView, error)
	RecallMessage(context.Context, string, string, string, time.Time) (model.MessageView, error)
	ListMessages(context.Context, string, string, model.MessageListCursor, time.Time, int) ([]model.MessageView, error)
	ListConversations(context.Context, string, model.ConversationListCursor, time.Time, int, bool) ([]model.ConversationSummary, error)
	UpdateConversationSettings(context.Context, string, string, model.ConversationSettingsUpdate, time.Time) (model.ConversationSettings, error)
	CurrentMessageSyncCursor(context.Context) (int64, error)
	MessageSyncCursorBeforeTime(context.Context, time.Time) (int64, error)
	ListMessagesSinceCursor(context.Context, string, int64, int64, int) ([]model.MessageSyncEntry, error)
	MarkConversationRead(context.Context, string, string, string, time.Time) (model.ReadThroughResult, error)
	DeleteMessagesForUser(context.Context, string, string, []string, time.Time) ([]string, error)
	FavoriteMessages(context.Context, string, string, []string, time.Time) ([]model.MessageFavorite, error)
	UnfavoriteMessage(context.Context, string, string) error
	ListMessageFavorites(context.Context, string, int) ([]model.MessageFavorite, error)
}

type MessageStickerCatalog interface {
	FindSticker(string) (model.Sticker, bool)
}

type MessageService struct {
	auth     *AuthService
	store    MessageStore
	stickers MessageStickerCatalog
	now      func() time.Time
}

type MessageInput struct {
	ConversationID string
	Type           string
	Body           string
	QuoteMessageID string
}

type MessageListFilter struct {
	ConversationID string
	Before         time.Time
	Cursor         string
	Limit          int
}

type MessageListPage struct {
	Items      []model.MessageView
	NextCursor string
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
	Since  time.Time
	Cursor string
	Limit  int
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

type MessageIDsInput struct {
	ConversationID string
	MessageIDs     []string
}

type ForwardInput struct {
	SourceConversationID string
	MessageIDs           []string
	TargetConversationID string
}

type FavoriteListFilter struct {
	Limit int
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

type messageListCursorPayload struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

type syncCursorPayload struct {
	ChangeID int64 `json:"changeId"`
}

const maxMessageBatchSize = 50

func NewMessageService(authService *AuthService, messageStore MessageStore) *MessageService {
	return NewMessageServiceWithStickers(authService, messageStore, NewStickerService())
}

func NewMessageServiceWithStickers(authService *AuthService, messageStore MessageStore, stickers MessageStickerCatalog) *MessageService {
	return &MessageService{
		auth:     authService,
		store:    messageStore,
		stickers: stickers,
		now:      time.Now,
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
	if messageType != model.MessageTypeText && messageType != model.MessageTypeSticker {
		return model.MessageView{}, ErrInvalidInput
	}

	body := strings.TrimSpace(input.Body)
	if err := s.validateMessageBody(messageType, body); err != nil {
		return model.MessageView{}, ErrInvalidInput
	}

	quoteMessageID := strings.TrimSpace(input.QuoteMessageID)
	if quoteMessageID != "" {
		quotedMessage, err := s.store.FindVisibleMessageByID(ctx, conversationID, actor.ID, quoteMessageID)
		if err != nil {
			return model.MessageView{}, mapMessageStoreError(err)
		}
		if quotedMessage.RecalledAt != nil {
			return model.MessageView{}, ErrConflict
		}
		quoteMessageID = quotedMessage.ID
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
		QuoteMessageID: quoteMessageID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.store.CreateMessage(ctx, message); err != nil {
		return model.MessageView{}, mapMessageStoreError(err)
	}

	return s.store.FindMessageByID(ctx, messageID)
}

func (s *MessageService) validateMessageBody(messageType, body string) error {
	switch messageType {
	case model.MessageTypeText:
		if body == "" || utf8.RuneCountInString(body) > 4000 {
			return ErrInvalidInput
		}
	case model.MessageTypeSticker:
		if body == "" || utf8.RuneCountInString(body) > 80 {
			return ErrInvalidInput
		}
		if s.stickers == nil {
			return ErrInvalidInput
		}
		if _, ok := s.stickers.FindSticker(body); !ok {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func (s *MessageService) ListMessages(ctx context.Context, token string, filter MessageListFilter) (MessageListPage, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return MessageListPage{}, err
	}

	conversationID := strings.TrimSpace(filter.ConversationID)
	if conversationID == "" {
		return MessageListPage{}, ErrInvalidInput
	}
	if filter.Limit < 0 || filter.Limit > 100 || (filter.Cursor != "" && !filter.Before.IsZero()) {
		return MessageListPage{}, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return MessageListPage{}, err
	}

	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	cursor, err := decodeMessageListCursor(filter.Cursor)
	if err != nil {
		return MessageListPage{}, ErrInvalidInput
	}

	messages, err := s.store.ListMessages(ctx, conversationID, actor.ID, cursor, filter.Before, limit+1)
	if err != nil {
		return MessageListPage{}, err
	}

	page := MessageListPage{Items: messages}
	if len(page.Items) > limit {
		page.Items = page.Items[len(page.Items)-limit:]
		nextCursor, err := encodeMessageListCursor(page.Items[0])
		if err != nil {
			return MessageListPage{}, err
		}
		page.NextCursor = nextCursor
	}
	return page, nil
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

func decodeMessageListCursor(raw string) (model.MessageListCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.MessageListCursor{}, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return model.MessageListCursor{}, err
	}

	var payload messageListCursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return model.MessageListCursor{}, err
	}
	payload.ID = strings.TrimSpace(payload.ID)
	if payload.ID == "" || payload.CreatedAt.IsZero() {
		return model.MessageListCursor{}, ErrInvalidInput
	}

	return model.MessageListCursor{
		Valid:     true,
		CreatedAt: payload.CreatedAt.UTC(),
		ID:        payload.ID,
	}, nil
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

func encodeMessageListCursor(message model.MessageView) (string, error) {
	payload := messageListCursorPayload{
		CreatedAt: message.CreatedAt.UTC(),
		ID:        message.ID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeSyncCursor(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}

	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return 0, err
	}

	var payload syncCursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, err
	}
	if payload.ChangeID < 0 {
		return 0, ErrInvalidInput
	}
	return payload.ChangeID, nil
}

func encodeSyncCursor(changeID int64) (string, error) {
	data, err := json.Marshal(syncCursorPayload{ChangeID: changeID})
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

	if filter.Limit < 0 || filter.Limit > 100 || (filter.Cursor != "" && !filter.Since.IsZero()) || (filter.Cursor == "" && filter.Since.IsZero()) {
		return model.SyncSnapshot{}, ErrInvalidInput
	}

	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	snapshotCursor, err := s.store.CurrentMessageSyncCursor(ctx)
	if err != nil {
		return model.SyncSnapshot{}, err
	}

	afterCursor := int64(0)
	if filter.Cursor != "" {
		afterCursor, err = decodeSyncCursor(filter.Cursor)
		if err != nil {
			return model.SyncSnapshot{}, ErrInvalidInput
		}
		if afterCursor > snapshotCursor {
			return model.SyncSnapshot{}, ErrInvalidInput
		}
	} else {
		afterCursor, err = s.store.MessageSyncCursorBeforeTime(ctx, filter.Since)
		if err != nil {
			return model.SyncSnapshot{}, err
		}
	}

	serverTime := s.now().UTC()
	conversations, err := s.store.ListConversations(ctx, actor.ID, model.ConversationListCursor{}, time.Time{}, limit, true)
	if err != nil {
		return model.SyncSnapshot{}, err
	}

	entries, err := s.store.ListMessagesSinceCursor(ctx, actor.ID, afterCursor, snapshotCursor, limit+1)
	if err != nil {
		return model.SyncSnapshot{}, err
	}

	snapshot := model.SyncSnapshot{
		ServerTime:    serverTime,
		Conversations: conversations,
	}

	if len(entries) > limit {
		snapshot.HasMore = true
		entries = entries[:limit]
		snapshot.NextCursor, err = encodeSyncCursor(entries[len(entries)-1].ChangeID)
		if err != nil {
			return model.SyncSnapshot{}, err
		}
	} else {
		snapshot.NextCursor, err = encodeSyncCursor(snapshotCursor)
		if err != nil {
			return model.SyncSnapshot{}, err
		}
	}

	snapshot.Messages = make([]model.MessageView, len(entries))
	for i := range entries {
		snapshot.Messages[i] = entries[i].Message
	}

	return snapshot, nil
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

	existingMessage, err := s.store.FindMessageByID(ctx, messageID)
	if err != nil {
		return model.MessageView{}, mapMessageStoreError(err)
	}
	if existingMessage.ConversationID != conversationID {
		return model.MessageView{}, ErrNotFound
	}
	if existingMessage.Sender.ID != actor.ID {
		return model.MessageView{}, ErrForbidden
	}
	if existingMessage.Type != model.MessageTypeText {
		return model.MessageView{}, ErrConflict
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

func (s *MessageService) ForwardMessages(ctx context.Context, token string, input ForwardInput) ([]model.MessageView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	sourceConversationID := strings.TrimSpace(input.SourceConversationID)
	targetConversationID := strings.TrimSpace(input.TargetConversationID)
	messageIDs, err := normalizeMessageIDs(input.MessageIDs)
	if err != nil || sourceConversationID == "" || targetConversationID == "" {
		return nil, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, sourceConversationID, actor.ID); err != nil {
		return nil, err
	}
	if err := s.requireConversationMember(ctx, targetConversationID, actor.ID); err != nil {
		return nil, err
	}

	now := s.now().UTC()
	forwarded := make([]model.MessageView, 0, len(messageIDs))
	for i, messageID := range messageIDs {
		sourceMessage, err := s.store.FindVisibleMessageByID(ctx, sourceConversationID, actor.ID, messageID)
		if err != nil {
			return nil, mapMessageStoreError(err)
		}
		if sourceMessage.RecalledAt != nil {
			return nil, ErrConflict
		}

		newMessageID, err := ids.New()
		if err != nil {
			return nil, err
		}
		createdAt := now.Add(time.Duration(i) * time.Millisecond)
		message := model.Message{
			ID:             newMessageID,
			ConversationID: targetConversationID,
			SenderID:       actor.ID,
			Type:           sourceMessage.Type,
			Body:           sourceMessage.Body,
			CreatedAt:      createdAt,
			UpdatedAt:      createdAt,
		}
		if err := s.store.CreateMessage(ctx, message); err != nil {
			return nil, mapMessageStoreError(err)
		}

		created, err := s.store.FindMessageByID(ctx, newMessageID)
		if err != nil {
			return nil, mapMessageStoreError(err)
		}
		forwarded = append(forwarded, created)
	}

	return forwarded, nil
}

func (s *MessageService) DeleteMessagesForMe(ctx context.Context, token string, input MessageIDsInput) ([]string, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	messageIDs, err := normalizeMessageIDs(input.MessageIDs)
	if err != nil || conversationID == "" {
		return nil, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return nil, err
	}

	deletedIDs, err := s.store.DeleteMessagesForUser(ctx, conversationID, actor.ID, messageIDs, s.now().UTC())
	if err != nil {
		return nil, mapMessageStoreError(err)
	}
	return deletedIDs, nil
}

func (s *MessageService) FavoriteMessages(ctx context.Context, token string, input MessageIDsInput) ([]model.MessageFavorite, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	messageIDs, err := normalizeMessageIDs(input.MessageIDs)
	if err != nil || conversationID == "" {
		return nil, ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return nil, err
	}
	for _, messageID := range messageIDs {
		message, err := s.store.FindVisibleMessageByID(ctx, conversationID, actor.ID, messageID)
		if err != nil {
			return nil, mapMessageStoreError(err)
		}
		if message.RecalledAt != nil {
			return nil, ErrConflict
		}
	}

	favorites, err := s.store.FavoriteMessages(ctx, conversationID, actor.ID, messageIDs, s.now().UTC())
	if err != nil {
		return nil, mapMessageStoreError(err)
	}
	return favorites, nil
}

func (s *MessageService) UnfavoriteMessage(ctx context.Context, token string, input MessageIDsInput) error {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	messageIDs, err := normalizeMessageIDs(input.MessageIDs)
	if err != nil || conversationID == "" || len(messageIDs) != 1 {
		return ErrInvalidInput
	}
	if err := s.requireConversationMember(ctx, conversationID, actor.ID); err != nil {
		return err
	}
	if _, err := s.store.FindVisibleMessageByID(ctx, conversationID, actor.ID, messageIDs[0]); err != nil {
		return mapMessageStoreError(err)
	}

	if err := s.store.UnfavoriteMessage(ctx, actor.ID, messageIDs[0]); err != nil {
		return mapMessageStoreError(err)
	}
	return nil
}

func (s *MessageService) ListMessageFavorites(ctx context.Context, token string, filter FavoriteListFilter) ([]model.MessageFavorite, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	if filter.Limit < 0 || filter.Limit > 100 {
		return nil, ErrInvalidInput
	}
	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}

	favorites, err := s.store.ListMessageFavorites(ctx, actor.ID, limit)
	if err != nil {
		return nil, err
	}
	return favorites, nil
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

func normalizeMessageIDs(rawIDs []string) ([]string, error) {
	if len(rawIDs) == 0 || len(rawIDs) > maxMessageBatchSize {
		return nil, ErrInvalidInput
	}

	seen := make(map[string]struct{}, len(rawIDs))
	messageIDs := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		messageID := strings.TrimSpace(rawID)
		if messageID == "" {
			return nil, ErrInvalidInput
		}
		if _, ok := seen[messageID]; ok {
			continue
		}
		seen[messageID] = struct{}{}
		messageIDs = append(messageIDs, messageID)
	}
	if len(messageIDs) == 0 {
		return nil, ErrInvalidInput
	}
	return messageIDs, nil
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
