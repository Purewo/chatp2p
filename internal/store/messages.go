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
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, sender_id, type, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, message.ID, message.ConversationID, message.SenderID, message.Type, message.Body, createdAt, updatedAt); err != nil {
		if isForeignKeyConstraint(err) {
			return ErrConversationNotFound
		}
		return fmt.Errorf("create message: %w", err)
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

func (s *SQLiteUserStore) ListMessages(ctx context.Context, conversationID string, before time.Time, limit int) ([]model.MessageView, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	beforeMillis := int64(0)
	if !before.IsZero() {
		beforeMillis = before.UTC().UnixMilli()
	}

	rows, err := s.db.QueryContext(ctx, messageViewSQL()+`
		WHERE m.conversation_id = ?
		  AND (? = 0 OR m.created_at < ?)
		ORDER BY m.created_at DESC, m.id DESC
		LIMIT ?
	`, conversationID, beforeMillis, beforeMillis, limit)
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

func (s *SQLiteUserStore) ListMessagesSince(ctx context.Context, userID string, after, before time.Time, limit int) ([]model.MessageView, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	afterMillis := after.UTC().UnixMilli()
	beforeMillis := int64(0)
	if !before.IsZero() {
		beforeMillis = before.UTC().UnixMilli()
	}

	rows, err := s.db.QueryContext(ctx, messageViewSQL()+`
		JOIN conversation_members cm ON cm.conversation_id = m.conversation_id
		WHERE cm.user_id = ?
		  AND m.updated_at > ?
		  AND (? = 0 OR m.updated_at <= ?)
		ORDER BY m.updated_at ASC, m.id ASC
		LIMIT ?
	`, userID, afterMillis, beforeMillis, beforeMillis, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages since: %w", err)
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

	return messages, nil
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
	lastMessages, err := s.loadConversationLastMessages(ctx, conversationIDs)
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
		ON CONFLICT(message_id, user_id) DO UPDATE SET read_at = excluded.read_at
		WHERE excluded.read_at > read_receipts.read_at
	`, userID, readAtMillis, conversationID, targetCreatedAt, userID)
	if err != nil {
		return model.ReadThroughResult{}, fmt.Errorf("mark conversation read: %w", err)
	}

	return model.ReadThroughResult{
		ConversationID:       conversationID,
		ReadThroughMessageID: messageID,
		ReadAt:               readAt.UTC(),
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

func (s *SQLiteUserStore) loadConversationLastMessages(ctx context.Context, conversationIDs []string) (map[string]model.MessageView, error) {
	lastMessages := make(map[string]model.MessageView, len(conversationIDs))
	if len(conversationIDs) == 0 {
		return lastMessages, nil
	}

	placeholders := make([]string, len(conversationIDs))
	args := make([]any, len(conversationIDs))
	for i, id := range conversationIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	rows, err := s.db.QueryContext(ctx, messageViewSQL()+`
		WHERE m.conversation_id IN (`+strings.Join(placeholders, ",")+`)
		  AND NOT EXISTS (
		    SELECT 1
		    FROM messages newer
		    WHERE newer.conversation_id = m.conversation_id
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
	args := make([]any, 0, len(conversationIDs)+2)
	for i, id := range conversationIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, userID, userID)

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
	return `
		SELECT m.id, m.conversation_id, m.type, m.body, m.created_at, m.updated_at,
		       sender.id, sender.username, sender.display_name, sender.avatar_url, sender.bio, sender.created_at, sender.updated_at,
		       m.edited_at, editor.id, editor.username, editor.display_name, editor.avatar_url, editor.bio, editor.created_at, editor.updated_at,
		       m.recalled_at, recalled.id, recalled.username, recalled.display_name, recalled.avatar_url, recalled.bio, recalled.created_at, recalled.updated_at
		FROM messages m
		JOIN users sender ON sender.id = m.sender_id
		LEFT JOIN users editor ON editor.id = m.edited_by
		LEFT JOIN users recalled ON recalled.id = m.recalled_by
	`
}

func scanMessageView(row scanner) (model.MessageView, error) {
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
	)
	if err := row.Scan(
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
	); err != nil {
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
