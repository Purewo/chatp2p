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

func (s *SQLiteUserStore) UpdateGroupConversationTitle(ctx context.Context, conversationID, title string, now time.Time) error {
	if err := s.requireGroupConversationType(ctx, conversationID); err != nil {
		return err
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE conversations
		SET title = ?, updated_at = ?
		WHERE id = ?
	`, title, now.UTC().Unix(), conversationID)
	if err != nil {
		return fmt.Errorf("update group conversation title: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrConversationNotFound
	}
	return nil
}

func (s *SQLiteUserStore) TransferGroupConversationOwner(ctx context.Context, conversationID, currentOwnerID, newOwnerID string, now time.Time) error {
	if err := s.requireGroupConversationType(ctx, conversationID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var one int
	if err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM conversation_members
		WHERE conversation_id = ? AND user_id = ?
	`, conversationID, newOwnerID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConversationMemberNotFound
		}
		return err
	}

	nowUnix := now.UTC().Unix()
	result, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET created_by = ?, updated_at = ?
		WHERE id = ? AND type = ? AND created_by = ?
	`, newOwnerID, nowUnix, conversationID, model.ConversationGroup, currentOwnerID)
	if err != nil {
		return fmt.Errorf("transfer group conversation owner: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrForbidden
	}

	if err := updateConversationMemberRole(ctx, tx, conversationID, currentOwnerID, "member"); err != nil {
		return err
	}
	if err := updateConversationMemberRole(ctx, tx, conversationID, newOwnerID, "owner"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteUserStore) AddConversationMembers(ctx context.Context, conversationID string, memberIDs []string, now time.Time) error {
	if len(memberIDs) == 0 {
		return nil
	}

	if err := s.requireGroupConversationType(ctx, conversationID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	nowUnix := now.UTC().Unix()
	memberPlaceholders := make([]string, len(memberIDs))
	memberArgs := make([]any, 0, len(memberIDs)*4+3)
	for i, memberID := range memberIDs {
		memberPlaceholders[i] = "(?, ?, 'member', ?)"
		memberArgs = append(memberArgs, conversationID, memberID, nowUnix)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at)
		VALUES `+strings.Join(memberPlaceholders, ",")+`
	`, memberArgs...); err != nil {
		if isForeignKeyConstraint(err) {
			return ErrUserNotFound
		}
		if isUniqueConstraint(err) {
			return ErrConversationMemberExists
		}
		return fmt.Errorf("add conversation members: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET updated_at = ?
		WHERE id = ? AND type = ?
	`, nowUnix, conversationID, model.ConversationGroup); err != nil {
		return fmt.Errorf("touch group conversation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (s *SQLiteUserStore) RemoveConversationMember(ctx context.Context, conversationID, userID string, now time.Time) error {
	if err := s.requireGroupConversationType(ctx, conversationID); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		DELETE FROM conversation_members
		WHERE conversation_id = ? AND user_id = ?
	`, conversationID, userID)
	if err != nil {
		return fmt.Errorf("remove conversation member: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrConversationMemberNotFound
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE conversations
		SET updated_at = ?
		WHERE id = ? AND type = ?
	`, now.UTC().Unix(), conversationID, model.ConversationGroup); err != nil {
		return fmt.Errorf("touch group conversation after remove: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func updateConversationMemberRole(ctx context.Context, tx *sql.Tx, conversationID, userID, role string) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE conversation_members
		SET role = ?
		WHERE conversation_id = ? AND user_id = ?
	`, role, conversationID, userID)
	if err != nil {
		return fmt.Errorf("update conversation member role: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrConversationMemberNotFound
	}
	return nil
}

func (s *SQLiteUserStore) requireGroupConversationType(ctx context.Context, conversationID string) error {
	row := s.db.QueryRowContext(ctx, `
		SELECT type
		FROM conversations
		WHERE id = ?
	`, conversationID)

	var conversationType string
	switch err := row.Scan(&conversationType); {
	case err == nil:
		if conversationType != model.ConversationGroup {
			return ErrForbidden
		}
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return ErrConversationNotFound
	default:
		return err
	}
}
