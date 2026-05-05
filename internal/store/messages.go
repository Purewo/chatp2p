package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatp2p/internal/model"
)

func (s *SQLiteUserStore) IsConversationMember(ctx context.Context, conversationID, userID string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM conversation_members
		WHERE conversation_id = ? AND user_id = ?
	`, conversationID, userID)

	var one int
	switch err := row.Scan(&one); {
	case err == nil:
		return true, nil
	case errors.Is(err, sql.ErrNoRows):
		return false, nil
	default:
		return false, err
	}
}

func (s *SQLiteUserStore) CreateMessage(ctx context.Context, message model.Message) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	createdAt := message.CreatedAt.UTC().UnixMilli()
	updatedAt := message.UpdatedAt.UTC().UnixMilli()
	var quoteMessageID any
	if message.QuoteMessageID != "" {
		quoteMessageID = message.QuoteMessageID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, sender_id, type, body, quoted_message_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, message.ID, message.ConversationID, message.SenderID, message.Type, message.Body, quoteMessageID, createdAt, updatedAt); err != nil {
		if isForeignKeyConstraint(err) {
			return ErrConversationNotFound
		}
		return fmt.Errorf("create message: %w", err)
	}

	if err := recordMessageChange(ctx, tx, message.ID, updatedAt); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET updated_at = ?
		WHERE id = ?
	`, message.UpdatedAt.UTC().Unix(), message.ConversationID); err != nil {
		return fmt.Errorf("touch conversation: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteUserStore) RecallMessage(ctx context.Context, conversationID, messageID, actorID string, now time.Time) (model.MessageView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.MessageView{}, err
	}
	defer tx.Rollback()

	var (
		targetConversationID string
		senderID             string
		recalledAt           sql.NullInt64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT conversation_id, sender_id, recalled_at
		FROM messages
		WHERE id = ?
	`, messageID).Scan(&targetConversationID, &senderID, &recalledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.MessageView{}, ErrMessageNotFound
		}
		return model.MessageView{}, err
	}

	if targetConversationID != conversationID {
		return model.MessageView{}, ErrMessageNotFound
	}
	if senderID != actorID {
		return model.MessageView{}, ErrForbidden
	}
	if recalledAt.Valid {
		return model.MessageView{}, ErrMessageAlreadyRecalled
	}

	now = now.UTC()
	nowMillis := now.UnixMilli()
	if _, err := tx.ExecContext(ctx, `
		UPDATE messages
		SET body = '', recalled_at = ?, recalled_by = ?, updated_at = ?
	WHERE id = ?
	`, nowMillis, actorID, nowMillis, messageID); err != nil {
		return model.MessageView{}, fmt.Errorf("recall message: %w", err)
	}

	if err := recordMessageChange(ctx, tx, messageID, nowMillis); err != nil {
		return model.MessageView{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET updated_at = ?
		WHERE id = ?
	`, now.Unix(), conversationID); err != nil {
		return model.MessageView{}, fmt.Errorf("touch conversation after message recall: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return model.MessageView{}, err
	}

	return s.FindMessageByID(ctx, messageID)
}

func (s *SQLiteUserStore) EditMessage(ctx context.Context, conversationID, messageID, actorID, body string, now time.Time) (model.MessageView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.MessageView{}, err
	}
	defer tx.Rollback()

	var (
		targetConversationID string
		senderID             string
		recalledAt           sql.NullInt64
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT conversation_id, sender_id, recalled_at
		FROM messages
		WHERE id = ?
	`, messageID).Scan(&targetConversationID, &senderID, &recalledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.MessageView{}, ErrMessageNotFound
		}
		return model.MessageView{}, err
	}

	if targetConversationID != conversationID {
		return model.MessageView{}, ErrMessageNotFound
	}
	if senderID != actorID {
		return model.MessageView{}, ErrForbidden
	}
	if recalledAt.Valid {
		return model.MessageView{}, ErrMessageAlreadyRecalled
	}

	now = now.UTC()
	nowMillis := now.UnixMilli()
	if _, err := tx.ExecContext(ctx, `
		UPDATE messages
		SET body = ?, edited_at = ?, edited_by = ?, updated_at = ?
	WHERE id = ?
	`, body, nowMillis, actorID, nowMillis, messageID); err != nil {
		return model.MessageView{}, fmt.Errorf("edit message: %w", err)
	}

	if err := recordMessageChange(ctx, tx, messageID, nowMillis); err != nil {
		return model.MessageView{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET updated_at = ?
		WHERE id = ?
	`, now.Unix(), conversationID); err != nil {
		return model.MessageView{}, fmt.Errorf("touch conversation after message edit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return model.MessageView{}, err
	}

	return s.FindMessageByID(ctx, messageID)
}

func (s *SQLiteUserStore) FindMessageByID(ctx context.Context, id string) (model.MessageView, error) {
	row := s.db.QueryRowContext(ctx, messageViewSQL()+`
		WHERE m.id = ?
	`, id)

	message, err := scanMessageView(row)
	if err != nil {
		return model.MessageView{}, err
	}

	receipts, err := s.loadReadReceipts(ctx, []string{message.ID})
	if err != nil {
		return model.MessageView{}, err
	}
	message.ReadBy = receipts[message.ID]
	return message, nil
}

func (s *SQLiteUserStore) FindVisibleMessageByID(ctx context.Context, conversationID, userID, messageID string) (model.MessageView, error) {
	row := s.db.QueryRowContext(ctx, messageViewSQL()+`
		WHERE m.id = ?
		  AND m.conversation_id = ?
		  AND NOT EXISTS (
		    SELECT 1
		    FROM message_user_states mus
		    WHERE mus.message_id = m.id
		      AND mus.user_id = ?
		      AND mus.deleted_at IS NOT NULL
		  )
	`, messageID, conversationID, userID)

	message, err := scanMessageView(row)
	if err != nil {
		return model.MessageView{}, err
	}

	receipts, err := s.loadReadReceipts(ctx, []string{message.ID})
	if err != nil {
		return model.MessageView{}, err
	}
	message.ReadBy = receipts[message.ID]
	return message, nil
}

func recordMessageChange(ctx context.Context, tx *sql.Tx, messageID string, changedAtMillis int64) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO message_changes (message_id, changed_at)
		VALUES (?, ?)
	`, messageID, changedAtMillis); err != nil {
		return fmt.Errorf("record message change: %w", err)
	}
	return nil
}

func (s *SQLiteUserStore) ListMessages(ctx context.Context, conversationID, userID string, cursor model.MessageListCursor, before time.Time, limit int) ([]model.MessageView, error) {
	if limit <= 0 {
		limit = 50
	}

	beforeMillis := int64(0)
	if !before.IsZero() {
		beforeMillis = before.UTC().UnixMilli()
	}
	cursorWhere, cursorArgs := messageListCursorWhere(cursor)
	args := []any{conversationID, beforeMillis, beforeMillis}
	args = append(args, cursorArgs...)
	args = append(args, userID, limit)

	rows, err := s.db.QueryContext(ctx, messageViewSQL()+`
		WHERE m.conversation_id = ?
		  AND (? = 0 OR m.created_at < ?)
		  `+cursorWhere+`
		  AND NOT EXISTS (
		    SELECT 1
		    FROM message_user_states mus
		    WHERE mus.message_id = m.id
		      AND mus.user_id = ?
		      AND mus.deleted_at IS NOT NULL
		  )
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messages []model.MessageView
	var messageIDs []string
	for rows.Next() {
		message, err := scanMessageView(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
		messageIDs = append(messageIDs, message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	receipts, err := s.loadReadReceipts(ctx, messageIDs)
	if err != nil {
		return nil, err
	}

	for i := range messages {
		messages[i].ReadBy = receipts[messages[i].ID]
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

func messageListCursorWhere(cursor model.MessageListCursor) (string, []any) {
	if !cursor.Valid {
		return "", nil
	}

	createdAt := cursor.CreatedAt.UTC().UnixMilli()
	return `
		  AND (
		    m.created_at < ?
		    OR (m.created_at = ? AND m.id < ?)
		  )`, []any{createdAt, createdAt, cursor.ID}
}

func (s *SQLiteUserStore) CurrentMessageSyncCursor(ctx context.Context) (int64, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(id), 0)
		FROM message_changes
	`)

	var cursor int64
	if err := row.Scan(&cursor); err != nil {
		return 0, fmt.Errorf("current message sync cursor: %w", err)
	}
	return cursor, nil
}

func (s *SQLiteUserStore) MessageSyncCursorBeforeTime(ctx context.Context, since time.Time) (int64, error) {
	if since.IsZero() {
		return 0, nil
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(id), 0)
		FROM message_changes
		WHERE changed_at < ?
	`, since.UTC().UnixMilli())

	var cursor int64
	if err := row.Scan(&cursor); err != nil {
		return 0, fmt.Errorf("message sync cursor before time: %w", err)
	}
	return cursor, nil
}

func (s *SQLiteUserStore) ListMessagesSinceCursor(ctx context.Context, userID string, afterCursor, beforeCursor int64, limit int) ([]model.MessageSyncEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH changed AS (
			SELECT mc.message_id, MAX(mc.id) AS change_id
			FROM message_changes mc
			JOIN messages src ON src.id = mc.message_id
			JOIN conversation_members cm ON cm.conversation_id = src.conversation_id
			WHERE cm.user_id = ?
			  AND mc.id > ?
			  AND (? = 0 OR mc.id <= ?)
			  AND NOT EXISTS (
			    SELECT 1
			    FROM message_user_states mus
			    WHERE mus.message_id = src.id
			      AND mus.user_id = ?
			      AND mus.deleted_at IS NOT NULL
			  )
			GROUP BY mc.message_id
			ORDER BY change_id ASC
			LIMIT ?
		)
	`+messageViewSQLWithExtraSelect("changed.change_id")+`
		JOIN changed ON changed.message_id = m.id
		ORDER BY changed.change_id ASC
	`, userID, afterCursor, beforeCursor, beforeCursor, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages since cursor: %w", err)
	}
	defer rows.Close()

	var entries []model.MessageSyncEntry
	var messageIDs []string
	for rows.Next() {
		entry, err := scanMessageSyncEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
		messageIDs = append(messageIDs, entry.Message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	receipts, err := s.loadReadReceipts(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Message.ReadBy = receipts[entries[i].Message.ID]
	}

	return entries, nil
}

func (s *SQLiteUserStore) ListConversations(ctx context.Context, userID string, cursor model.ConversationListCursor, before time.Time, limit int, includeArchived bool) ([]model.ConversationSummary, error) {
	if limit <= 0 {
		limit = 50
	}

	beforeSeconds := int64(0)
	if !before.IsZero() {
		beforeSeconds = before.UTC().Unix()
	}
	includeArchivedValue := 0
	if includeArchived {
		includeArchivedValue = 1
	}

	cursorWhere, cursorArgs := conversationListCursorWhere(cursor)
	args := []any{userID, beforeSeconds, beforeSeconds, includeArchivedValue}
	args = append(args, cursorArgs...)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.type, c.title, c.created_by, c.created_at, c.updated_at,
		       self.pinned_at, self.muted_until, self.archived_at
		FROM conversations c
		JOIN conversation_members self ON self.conversation_id = c.id
		WHERE self.user_id = ?
		  AND (? = 0 OR c.updated_at < ?)
		  AND (? = 1 OR self.archived_at IS NULL)
		  `+cursorWhere+`
		ORDER BY (self.pinned_at IS NOT NULL) DESC, self.pinned_at DESC, c.updated_at DESC, c.id DESC
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []model.ConversationSummary
	var conversationIDs []string
	for rows.Next() {
		var (
			conversation          model.ConversationSummary
			conversationCreatedAt int64
			conversationUpdatedAt int64
			pinnedAt              sql.NullInt64
			mutedUntil            sql.NullInt64
			archivedAt            sql.NullInt64
		)
		if err := rows.Scan(
			&conversation.ID,
			&conversation.Type,
			&conversation.Title,
			&conversation.CreatedBy,
			&conversationCreatedAt,
			&conversationUpdatedAt,
			&pinnedAt,
			&mutedUntil,
			&archivedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation summary: %w", err)
		}
		conversation.CreatedAt = time.Unix(conversationCreatedAt, 0).UTC()
		conversation.UpdatedAt = time.Unix(conversationUpdatedAt, 0).UTC()
		conversation.PinnedAt = unixSecondsPtr(pinnedAt)
		conversation.MutedUntil = unixSecondsPtr(mutedUntil)
		conversation.ArchivedAt = unixSecondsPtr(archivedAt)
		conversations = append(conversations, conversation)
		conversationIDs = append(conversationIDs, conversation.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(conversations) == 0 {
		return []model.ConversationSummary{}, nil
	}

	members, err := s.loadConversationMembers(ctx, conversationIDs)
	if err != nil {
		return nil, err
	}
	lastMessages, err := s.loadConversationLastMessages(ctx, userID, conversationIDs)
	if err != nil {
		return nil, err
	}
	unreadCounts, err := s.loadConversationUnreadCounts(ctx, userID, conversationIDs)
	if err != nil {
		return nil, err
	}

	for i := range conversations {
		conversation := &conversations[i]
		conversation.Members = members[conversation.ID]
		conversation.Title = conversationSummaryTitle(*conversation, userID)
		if message, ok := lastMessages[conversation.ID]; ok {
			msg := message
			conversation.LastMessage = &msg
		}
		conversation.UnreadCount = unreadCounts[conversation.ID]
	}

	return conversations, nil
}

func conversationListCursorWhere(cursor model.ConversationListCursor) (string, []any) {
	if !cursor.Valid {
		return "", nil
	}

	if cursor.PinnedAt != nil {
		pinnedAt := cursor.PinnedAt.UTC().Unix()
		updatedAt := cursor.UpdatedAt.UTC().Unix()
		return `
		  AND (
		    (self.pinned_at IS NOT NULL AND self.pinned_at < ?)
		    OR (self.pinned_at IS NOT NULL AND self.pinned_at = ? AND c.updated_at < ?)
		    OR (self.pinned_at IS NOT NULL AND self.pinned_at = ? AND c.updated_at = ? AND c.id < ?)
		    OR self.pinned_at IS NULL
		  )`, []any{pinnedAt, pinnedAt, updatedAt, pinnedAt, updatedAt, cursor.ID}
	}

	updatedAt := cursor.UpdatedAt.UTC().Unix()
	return `
	  AND self.pinned_at IS NULL
	  AND (c.updated_at < ? OR (c.updated_at = ? AND c.id < ?))`, []any{updatedAt, updatedAt, cursor.ID}
}

func (s *SQLiteUserStore) UpdateConversationSettings(ctx context.Context, conversationID, userID string, update model.ConversationSettingsUpdate, now time.Time) (model.ConversationSettings, error) {
	setClauses := []string{}
	args := []any{}
	nowUnix := now.UTC().Unix()

	if update.Pinned != nil {
		if *update.Pinned {
			setClauses = append(setClauses, "pinned_at = ?")
			args = append(args, nowUnix)
		} else {
			setClauses = append(setClauses, "pinned_at = NULL")
		}
	}
	if update.UpdateMutedUntil {
		if update.MutedUntil != nil {
			setClauses = append(setClauses, "muted_until = ?")
			args = append(args, update.MutedUntil.UTC().Unix())
		} else {
			setClauses = append(setClauses, "muted_until = NULL")
		}
	}
	if update.Archived != nil {
		if *update.Archived {
			setClauses = append(setClauses, "archived_at = ?")
			args = append(args, nowUnix)
		} else {
			setClauses = append(setClauses, "archived_at = NULL")
		}
	}
	if len(setClauses) == 0 {
		return s.loadConversationSettings(ctx, conversationID, userID)
	}

	args = append(args, conversationID, userID)
	result, err := s.db.ExecContext(ctx, `
		UPDATE conversation_members
		SET `+strings.Join(setClauses, ", ")+`
		WHERE conversation_id = ? AND user_id = ?
	`, args...)
	if err != nil {
		return model.ConversationSettings{}, fmt.Errorf("update conversation settings: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return model.ConversationSettings{}, err
	}
	if rowsAffected == 0 {
		return model.ConversationSettings{}, ErrConversationMemberNotFound
	}

	return s.loadConversationSettings(ctx, conversationID, userID)
}

func (s *SQLiteUserStore) loadConversationSettings(ctx context.Context, conversationID, userID string) (model.ConversationSettings, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT conversation_id, pinned_at, muted_until, archived_at
		FROM conversation_members
		WHERE conversation_id = ? AND user_id = ?
	`, conversationID, userID)

	var (
		settings   model.ConversationSettings
		pinnedAt   sql.NullInt64
		mutedUntil sql.NullInt64
		archivedAt sql.NullInt64
	)
	if err := row.Scan(&settings.ConversationID, &pinnedAt, &mutedUntil, &archivedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ConversationSettings{}, ErrConversationMemberNotFound
		}
		return model.ConversationSettings{}, fmt.Errorf("load conversation settings: %w", err)
	}
	settings.PinnedAt = unixSecondsPtr(pinnedAt)
	settings.MutedUntil = unixSecondsPtr(mutedUntil)
	settings.ArchivedAt = unixSecondsPtr(archivedAt)
	return settings, nil
}

func conversationSummaryTitle(conversation model.ConversationSummary, userID string) string {
	if conversation.Title != "" || conversation.Type != model.ConversationDirect {
		return conversation.Title
	}
	for _, member := range conversation.Members {
		if member.ID != userID {
			if member.DisplayName != "" {
				return member.DisplayName
			}
			return member.Username
		}
	}
	return conversation.Title
}

func unixSecondsPtr(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	t := time.Unix(value.Int64, 0).UTC()
	return &t
}

func (s *SQLiteUserStore) MarkConversationRead(ctx context.Context, conversationID, userID, messageID string, readAt time.Time) (model.ReadThroughResult, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT conversation_id, created_at
		FROM messages
		WHERE id = ?
	`, messageID)

	var targetConversationID string
	var targetCreatedAt int64
	if err := row.Scan(&targetConversationID, &targetCreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ReadThroughResult{}, ErrMessageNotFound
		}
		return model.ReadThroughResult{}, err
	}
	if targetConversationID != conversationID {
		return model.ReadThroughResult{}, ErrMessageNotFound
	}

	readAtMillis := readAt.UTC().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO read_receipts (message_id, user_id, read_at)
		SELECT id, ?, ?
		FROM messages
		WHERE conversation_id = ?
		  AND created_at <= ?
		  AND sender_id <> ?
		  AND NOT EXISTS (
		    SELECT 1
		    FROM message_user_states mus
		    WHERE mus.message_id = messages.id
		      AND mus.user_id = ?
		      AND mus.deleted_at IS NOT NULL
		  )
		ON CONFLICT(message_id, user_id) DO UPDATE SET read_at = excluded.read_at
		WHERE excluded.read_at > read_receipts.read_at
	`, userID, readAtMillis, conversationID, targetCreatedAt, userID, userID)
	if err != nil {
		return model.ReadThroughResult{}, fmt.Errorf("mark conversation read: %w", err)
	}

	return model.ReadThroughResult{
		ConversationID:       conversationID,
		ReadThroughMessageID: messageID,
		ReadAt:               readAt.UTC(),
	}, nil
}

func (s *SQLiteUserStore) DeleteMessagesForUser(ctx context.Context, conversationID, userID string, messageIDs []string, now time.Time) ([]string, error) {
	if len(messageIDs) == 0 {
		return []string{}, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	deletedAt := now.UTC().UnixMilli()
	for _, messageID := range messageIDs {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO message_user_states (message_id, user_id, deleted_at)
			SELECT m.id, ?, ?
			FROM messages m
			WHERE m.id = ?
			  AND m.conversation_id = ?
			ON CONFLICT(message_id, user_id) DO UPDATE SET deleted_at = excluded.deleted_at
		`, userID, deletedAt, messageID, conversationID)
		if err != nil {
			return nil, fmt.Errorf("delete message for user: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			return nil, ErrMessageNotFound
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return append([]string(nil), messageIDs...), nil
}

func (s *SQLiteUserStore) FavoriteMessages(ctx context.Context, conversationID, userID string, messageIDs []string, now time.Time) ([]model.MessageFavorite, error) {
	if len(messageIDs) == 0 {
		return []model.MessageFavorite{}, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	favoritedAt := now.UTC().UnixMilli()
	for _, messageID := range messageIDs {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO message_user_states (message_id, user_id, favorited_at)
			SELECT m.id, ?, ?
			FROM messages m
			WHERE m.id = ?
			  AND m.conversation_id = ?
			ON CONFLICT(message_id, user_id) DO UPDATE SET favorited_at = excluded.favorited_at
		`, userID, favoritedAt, messageID, conversationID)
		if err != nil {
			return nil, fmt.Errorf("favorite message: %w", err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if rowsAffected == 0 {
			return nil, ErrMessageNotFound
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	favorites := make([]model.MessageFavorite, 0, len(messageIDs))
	for _, messageID := range messageIDs {
		favorite, err := s.loadMessageFavorite(ctx, userID, messageID)
		if err != nil {
			return nil, err
		}
		favorites = append(favorites, favorite)
	}
	return favorites, nil
}

func (s *SQLiteUserStore) UnfavoriteMessage(ctx context.Context, userID, messageID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		UPDATE message_user_states
		SET favorited_at = NULL
		WHERE message_id = ?
		  AND user_id = ?
	`, messageID, userID); err != nil {
		return fmt.Errorf("unfavorite message: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM message_user_states
		WHERE message_id = ?
		  AND user_id = ?
		  AND favorited_at IS NULL
		  AND deleted_at IS NULL
	`, messageID, userID); err != nil {
		return fmt.Errorf("cleanup message user state: %w", err)
	}

	return tx.Commit()
}

func (s *SQLiteUserStore) ListMessageFavorites(ctx context.Context, userID string, limit int) ([]model.MessageFavorite, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx, messageViewSQLWithExtraSelect("mus.favorited_at")+`
		JOIN message_user_states mus ON mus.message_id = m.id
		JOIN conversation_members cm ON cm.conversation_id = m.conversation_id AND cm.user_id = mus.user_id
		WHERE mus.user_id = ?
		  AND mus.favorited_at IS NOT NULL
		  AND mus.deleted_at IS NULL
		ORDER BY mus.favorited_at DESC, m.created_at DESC, m.id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list message favorites: %w", err)
	}
	defer rows.Close()

	var favorites []model.MessageFavorite
	var messageIDs []string
	for rows.Next() {
		var favoritedAt int64
		message, err := scanMessageViewWithExtras(rows, &favoritedAt)
		if err != nil {
			return nil, err
		}
		favorites = append(favorites, model.MessageFavorite{
			Message:     message,
			FavoritedAt: time.UnixMilli(favoritedAt).UTC(),
		})
		messageIDs = append(messageIDs, message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	receipts, err := s.loadReadReceipts(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for i := range favorites {
		favorites[i].Message.ReadBy = receipts[favorites[i].Message.ID]
	}

	return favorites, nil
}

func (s *SQLiteUserStore) loadMessageFavorite(ctx context.Context, userID, messageID string) (model.MessageFavorite, error) {
	row := s.db.QueryRowContext(ctx, messageViewSQLWithExtraSelect("mus.favorited_at")+`
		JOIN message_user_states mus ON mus.message_id = m.id
		WHERE mus.user_id = ?
		  AND m.id = ?
		  AND mus.favorited_at IS NOT NULL
		  AND mus.deleted_at IS NULL
	`, userID, messageID)

	var favoritedAt int64
	message, err := scanMessageViewWithExtras(row, &favoritedAt)
	if err != nil {
		return model.MessageFavorite{}, err
	}

	receipts, err := s.loadReadReceipts(ctx, []string{message.ID})
	if err != nil {
		return model.MessageFavorite{}, err
	}
	message.ReadBy = receipts[message.ID]

	return model.MessageFavorite{
		Message:     message,
		FavoritedAt: time.UnixMilli(favoritedAt).UTC(),
	}, nil
}

func (s *SQLiteUserStore) loadConversationMembers(ctx context.Context, conversationIDs []string) (map[string][]model.Profile, error) {
	members := make(map[string][]model.Profile, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return members, nil
	}

	placeholders := make([]string, len(conversationIDs))
	args := make([]any, len(conversationIDs))
	for i, id := range conversationIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT cm.conversation_id,
		       u.id, u.username, u.display_name, u.avatar_url, u.bio, u.created_at, u.updated_at
		FROM conversation_members cm
		JOIN users u ON u.id = cm.user_id
		WHERE cm.conversation_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY cm.conversation_id, u.username
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load conversation members: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			conversationID string
			profile        model.Profile
			createdAt      int64
			updatedAt      int64
		)
		if err := rows.Scan(
			&conversationID,
			&profile.ID,
			&profile.Username,
			&profile.DisplayName,
			&profile.AvatarURL,
			&profile.Bio,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan conversation member: %w", err)
		}
		profile.CreatedAt = time.Unix(createdAt, 0).UTC()
		profile.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		members[conversationID] = append(members[conversationID], profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return members, nil
}

func (s *SQLiteUserStore) loadConversationLastMessages(ctx context.Context, userID string, conversationIDs []string) (map[string]model.MessageView, error) {
	lastMessages := make(map[string]model.MessageView, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return lastMessages, nil
	}

	placeholders := make([]string, len(conversationIDs))
	args := make([]any, 0, len(conversationIDs)+2)
	for i, id := range conversationIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, userID, userID)

	rows, err := s.db.QueryContext(ctx, messageViewSQL()+`
		WHERE m.conversation_id IN (`+strings.Join(placeholders, ",")+`)
		  AND NOT EXISTS (
		    SELECT 1
		    FROM message_user_states hidden
		    WHERE hidden.message_id = m.id
		      AND hidden.user_id = ?
		      AND hidden.deleted_at IS NOT NULL
		  )
		  AND NOT EXISTS (
		    SELECT 1
		    FROM messages newer
		    WHERE newer.conversation_id = m.conversation_id
		      AND NOT EXISTS (
		        SELECT 1
		        FROM message_user_states newer_hidden
		        WHERE newer_hidden.message_id = newer.id
		          AND newer_hidden.user_id = ?
		          AND newer_hidden.deleted_at IS NOT NULL
		      )
		      AND (
		        newer.created_at > m.created_at
		        OR (newer.created_at = m.created_at AND newer.id > m.id)
		      )
		  )
		ORDER BY m.conversation_id, m.created_at DESC, m.id DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load conversation last messages: %w", err)
	}
	defer rows.Close()

	var messageIDs []string
	for rows.Next() {
		message, err := scanMessageView(rows)
		if err != nil {
			return nil, err
		}
		lastMessages[message.ConversationID] = message
		messageIDs = append(messageIDs, message.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	receipts, err := s.loadReadReceipts(ctx, messageIDs)
	if err != nil {
		return nil, err
	}
	for conversationID, message := range lastMessages {
		message.ReadBy = receipts[message.ID]
		lastMessages[conversationID] = message
	}

	return lastMessages, nil
}

func (s *SQLiteUserStore) loadConversationUnreadCounts(ctx context.Context, userID string, conversationIDs []string) (map[string]int, error) {
	counts := make(map[string]int, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return counts, nil
	}

	placeholders := make([]string, len(conversationIDs))
	args := make([]any, 0, len(conversationIDs)+3)
	for i, id := range conversationIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, userID, userID, userID)

	rows, err := s.db.QueryContext(ctx, `
		SELECT m.conversation_id, COUNT(*)
		FROM messages m
		WHERE m.conversation_id IN (`+strings.Join(placeholders, ",")+`)
		  AND m.sender_id <> ?
		  AND m.recalled_at IS NULL
		  AND NOT EXISTS (
		    SELECT 1
		    FROM read_receipts rr
		    WHERE rr.message_id = m.id
		      AND rr.user_id = ?
		  )
		  AND NOT EXISTS (
		    SELECT 1
		    FROM message_user_states mus
		    WHERE mus.message_id = m.id
		      AND mus.user_id = ?
		      AND mus.deleted_at IS NOT NULL
		  )
		GROUP BY m.conversation_id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load conversation unread counts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var conversationID string
		var count int
		if err := rows.Scan(&conversationID, &count); err != nil {
			return nil, fmt.Errorf("scan unread count: %w", err)
		}
		counts[conversationID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

func messageViewSQL() string {
	return messageViewSQLWithExtraSelect("")
}

func messageViewSQLWithExtraSelect(extraSelect string) string {
	extra := ""
	if extraSelect != "" {
		extra = ", " + extraSelect
	}
	return `
		SELECT m.id, m.conversation_id, m.type, m.body, m.created_at, m.updated_at,
		       sender.id, sender.username, sender.display_name, sender.avatar_url, sender.bio, sender.created_at, sender.updated_at,
		       m.edited_at, editor.id, editor.username, editor.display_name, editor.avatar_url, editor.bio, editor.created_at, editor.updated_at,
		       m.recalled_at, recalled.id, recalled.username, recalled.display_name, recalled.avatar_url, recalled.bio, recalled.created_at, recalled.updated_at,
		       quoted.id, quoted.conversation_id, quoted.type, quoted.body, quoted.created_at, quoted.recalled_at,
		       quote_sender.id, quote_sender.username, quote_sender.display_name, quote_sender.avatar_url, quote_sender.bio, quote_sender.created_at, quote_sender.updated_at` + extra + `
		FROM messages m
		JOIN users sender ON sender.id = m.sender_id
		LEFT JOIN users editor ON editor.id = m.edited_by
		LEFT JOIN users recalled ON recalled.id = m.recalled_by
		LEFT JOIN messages quoted ON quoted.id = m.quoted_message_id
		LEFT JOIN users quote_sender ON quote_sender.id = quoted.sender_id
	`
}

func scanMessageView(row scanner) (model.MessageView, error) {
	return scanMessageViewWithExtras(row)
}

func scanMessageSyncEntry(row scanner) (model.MessageSyncEntry, error) {
	var changeID int64
	message, err := scanMessageViewWithExtras(row, &changeID)
	if err != nil {
		return model.MessageSyncEntry{}, err
	}
	return model.MessageSyncEntry{
		ChangeID: changeID,
		Message:  message,
	}, nil
}

func scanMessageViewWithExtras(row scanner, extras ...any) (model.MessageView, error) {
	var (
		message          model.MessageView
		messageCreatedAt int64
		messageUpdatedAt int64
		senderCreatedAt  int64
		senderUpdatedAt  int64
		editedAt         sql.NullInt64
		editorID         sql.NullString
		editorUsername   sql.NullString
		editorName       sql.NullString
		editorAvatar     sql.NullString
		editorBio        sql.NullString
		editorCreated    sql.NullInt64
		editorUpdated    sql.NullInt64
		recalledAt       sql.NullInt64
		recalledID       sql.NullString
		recalledUsername sql.NullString
		recalledName     sql.NullString
		recalledAvatar   sql.NullString
		recalledBio      sql.NullString
		recalledCreated  sql.NullInt64
		recalledUpdated  sql.NullInt64
		quoteID          sql.NullString
		quoteConvID      sql.NullString
		quoteType        sql.NullString
		quoteBody        sql.NullString
		quoteCreatedAt   sql.NullInt64
		quoteRecalledAt  sql.NullInt64
		quoteSenderID    sql.NullString
		quoteUsername    sql.NullString
		quoteName        sql.NullString
		quoteAvatar      sql.NullString
		quoteBio         sql.NullString
		quoteCreated     sql.NullInt64
		quoteUpdated     sql.NullInt64
	)
	scanArgs := []any{
		&message.ID,
		&message.ConversationID,
		&message.Type,
		&message.Body,
		&messageCreatedAt,
		&messageUpdatedAt,
		&message.Sender.ID,
		&message.Sender.Username,
		&message.Sender.DisplayName,
		&message.Sender.AvatarURL,
		&message.Sender.Bio,
		&senderCreatedAt,
		&senderUpdatedAt,
		&editedAt,
		&editorID,
		&editorUsername,
		&editorName,
		&editorAvatar,
		&editorBio,
		&editorCreated,
		&editorUpdated,
		&recalledAt,
		&recalledID,
		&recalledUsername,
		&recalledName,
		&recalledAvatar,
		&recalledBio,
		&recalledCreated,
		&recalledUpdated,
		&quoteID,
		&quoteConvID,
		&quoteType,
		&quoteBody,
		&quoteCreatedAt,
		&quoteRecalledAt,
		&quoteSenderID,
		&quoteUsername,
		&quoteName,
		&quoteAvatar,
		&quoteBio,
		&quoteCreated,
		&quoteUpdated,
	}
	scanArgs = append(scanArgs, extras...)
	if err := row.Scan(scanArgs...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.MessageView{}, ErrMessageNotFound
		}
		return model.MessageView{}, fmt.Errorf("scan message: %w", err)
	}

	message.CreatedAt = time.UnixMilli(messageCreatedAt).UTC()
	message.UpdatedAt = time.UnixMilli(messageUpdatedAt).UTC()
	message.Sender.CreatedAt = time.Unix(senderCreatedAt, 0).UTC()
	message.Sender.UpdatedAt = time.Unix(senderUpdatedAt, 0).UTC()
	if editedAt.Valid {
		value := time.UnixMilli(editedAt.Int64).UTC()
		message.EditedAt = &value
	}
	if editorID.Valid {
		profile := model.Profile{
			ID:          editorID.String,
			Username:    editorUsername.String,
			DisplayName: editorName.String,
			AvatarURL:   editorAvatar.String,
			Bio:         editorBio.String,
		}
		if editorCreated.Valid {
			profile.CreatedAt = time.Unix(editorCreated.Int64, 0).UTC()
		}
		if editorUpdated.Valid {
			profile.UpdatedAt = time.Unix(editorUpdated.Int64, 0).UTC()
		}
		message.EditedBy = &profile
	}
	if recalledAt.Valid {
		value := time.UnixMilli(recalledAt.Int64).UTC()
		message.RecalledAt = &value
	}
	if recalledID.Valid {
		profile := model.Profile{
			ID:          recalledID.String,
			Username:    recalledUsername.String,
			DisplayName: recalledName.String,
			AvatarURL:   recalledAvatar.String,
			Bio:         recalledBio.String,
		}
		if recalledCreated.Valid {
			profile.CreatedAt = time.Unix(recalledCreated.Int64, 0).UTC()
		}
		if recalledUpdated.Valid {
			profile.UpdatedAt = time.Unix(recalledUpdated.Int64, 0).UTC()
		}
		message.RecalledBy = &profile
	}
	if quoteID.Valid {
		quotedMessage := model.QuotedMessage{
			ID:             quoteID.String,
			ConversationID: quoteConvID.String,
			Type:           quoteType.String,
			Body:           quoteBody.String,
			Sender: model.Profile{
				ID:          quoteSenderID.String,
				Username:    quoteUsername.String,
				DisplayName: quoteName.String,
				AvatarURL:   quoteAvatar.String,
				Bio:         quoteBio.String,
			},
		}
		if quoteCreatedAt.Valid {
			quotedMessage.CreatedAt = time.UnixMilli(quoteCreatedAt.Int64).UTC()
		}
		if quoteRecalledAt.Valid {
			value := time.UnixMilli(quoteRecalledAt.Int64).UTC()
			quotedMessage.RecalledAt = &value
		}
		if quoteCreated.Valid {
			quotedMessage.Sender.CreatedAt = time.Unix(quoteCreated.Int64, 0).UTC()
		}
		if quoteUpdated.Valid {
			quotedMessage.Sender.UpdatedAt = time.Unix(quoteUpdated.Int64, 0).UTC()
		}
		message.QuotedMessage = &quotedMessage
	}
	return message, nil
}

func (s *SQLiteUserStore) loadReadReceipts(ctx context.Context, messageIDs []string) (map[string][]model.ReadReceipt, error) {
	receipts := make(map[string][]model.ReadReceipt, len(messageIDs))
	if len(messageIDs) == 0 {
		return receipts, nil
	}
	for _, id := range messageIDs {
		receipts[id] = []model.ReadReceipt{}
	}

	placeholders := make([]string, len(messageIDs))
	args := make([]any, len(messageIDs))
	for i, id := range messageIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT rr.message_id,
		       u.id, u.username, u.display_name, u.avatar_url, u.bio, u.created_at, u.updated_at,
		       rr.read_at
		FROM read_receipts rr
		JOIN users u ON u.id = rr.user_id
		WHERE rr.message_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY rr.read_at ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load read receipts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			messageID string
			receipt   model.ReadReceipt
			createdAt int64
			updatedAt int64
			readAt    int64
		)
		if err := rows.Scan(
			&messageID,
			&receipt.User.ID,
			&receipt.User.Username,
			&receipt.User.DisplayName,
			&receipt.User.AvatarURL,
			&receipt.User.Bio,
			&createdAt,
			&updatedAt,
			&readAt,
		); err != nil {
			return nil, fmt.Errorf("scan read receipt: %w", err)
		}
		receipt.User.CreatedAt = time.Unix(createdAt, 0).UTC()
		receipt.User.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		receipt.ReadAt = time.UnixMilli(readAt).UTC()
		receipts[messageID] = append(receipts[messageID], receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return receipts, nil
}
