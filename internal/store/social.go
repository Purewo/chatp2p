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

func (s *SQLiteUserStore) SearchUsers(ctx context.Context, actorID, query string, limit int) ([]model.Profile, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	pattern := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, display_name, avatar_url, bio, created_at, updated_at
		FROM users
		WHERE id <> ?
		  AND (LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?)
		ORDER BY username
		LIMIT ?
	`, actorID, pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()

	var profiles []model.Profile
	for rows.Next() {
		profile, err := scanProfileRows(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *SQLiteUserStore) CreateFriendRequest(ctx context.Context, request model.FriendRequest) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO friend_requests (id, requester_id, addressee_id, message, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, request.ID, request.RequesterID, request.AddresseeID, request.Message, request.Status, request.CreatedAt.Unix(), request.UpdatedAt.Unix())
	if err != nil {
		if isUniqueConstraint(err) {
			return ErrFriendRequestExists
		}
		if isForeignKeyConstraint(err) {
			return ErrUserNotFound
		}
		return fmt.Errorf("create friend request: %w", err)
	}
	return nil
}

func (s *SQLiteUserStore) FindFriendRequestByID(ctx context.Context, id string) (model.FriendRequestView, error) {
	row := s.db.QueryRowContext(ctx, friendRequestViewSQL()+`
		WHERE fr.id = ?
	`, id)
	return scanFriendRequestView(row)
}

func (s *SQLiteUserStore) ListFriendRequests(ctx context.Context, userID, box, status string) ([]model.FriendRequestView, error) {
	where := "fr.status = ? AND fr.addressee_id = ?"
	args := []any{status, userID}

	switch box {
	case "outgoing":
		where = "fr.status = ? AND fr.requester_id = ?"
	case "all":
		where = "fr.status = ? AND (fr.requester_id = ? OR fr.addressee_id = ?)"
		args = []any{status, userID, userID}
	}

	rows, err := s.db.QueryContext(ctx, friendRequestViewSQL()+`
		WHERE `+where+`
		ORDER BY fr.updated_at DESC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list friend requests: %w", err)
	}
	defer rows.Close()

	var requests []model.FriendRequestView
	for rows.Next() {
		request, err := scanFriendRequestViewRows(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return requests, nil
}

func (s *SQLiteUserStore) AreFriends(ctx context.Context, userID, friendID string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM friendships
		WHERE user_id = ? AND friend_id = ?
	`, userID, friendID)

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

func (s *SQLiteUserStore) HasPendingFriendRequestBetween(ctx context.Context, userID, otherUserID string) (bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM friend_requests
		WHERE status = ?
		  AND (
		    (requester_id = ? AND addressee_id = ?)
		    OR (requester_id = ? AND addressee_id = ?)
		  )
	`, model.FriendRequestPending, userID, otherUserID, otherUserID, userID)

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

func (s *SQLiteUserStore) AcceptFriendRequest(ctx context.Context, requestID, actorID string, now time.Time) (model.FriendRequestView, error) {
	request, err := s.findRawFriendRequest(ctx, requestID)
	if err != nil {
		return model.FriendRequestView{}, err
	}
	if request.AddresseeID != actorID {
		return model.FriendRequestView{}, ErrForbidden
	}
	if request.Status != model.FriendRequestPending {
		return model.FriendRequestView{}, ErrInvalidFriendRequestState
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.FriendRequestView{}, err
	}
	defer tx.Rollback()

	nowUnix := now.UTC().Unix()
	if _, err := tx.ExecContext(ctx, `
		UPDATE friend_requests
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, model.FriendRequestAccepted, nowUnix, requestID); err != nil {
		return model.FriendRequestView{}, fmt.Errorf("accept friend request: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO friendships (user_id, friend_id, created_at)
		VALUES (?, ?, ?), (?, ?, ?)
	`, request.RequesterID, request.AddresseeID, nowUnix, request.AddresseeID, request.RequesterID, nowUnix); err != nil {
		return model.FriendRequestView{}, fmt.Errorf("create friendship: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return model.FriendRequestView{}, err
	}

	return s.FindFriendRequestByID(ctx, requestID)
}

func (s *SQLiteUserStore) DeclineFriendRequest(ctx context.Context, requestID, actorID string, now time.Time) (model.FriendRequestView, error) {
	request, err := s.findRawFriendRequest(ctx, requestID)
	if err != nil {
		return model.FriendRequestView{}, err
	}
	if request.AddresseeID != actorID {
		return model.FriendRequestView{}, ErrForbidden
	}
	if request.Status != model.FriendRequestPending {
		return model.FriendRequestView{}, ErrInvalidFriendRequestState
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE friend_requests
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, model.FriendRequestDeclined, now.UTC().Unix(), requestID)
	if err != nil {
		return model.FriendRequestView{}, fmt.Errorf("decline friend request: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return model.FriendRequestView{}, err
	}
	if rowsAffected == 0 {
		return model.FriendRequestView{}, ErrFriendRequestNotFound
	}

	return s.FindFriendRequestByID(ctx, requestID)
}

func (s *SQLiteUserStore) ListFriends(ctx context.Context, userID string) ([]model.Friend, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.username, u.display_name, u.avatar_url, u.bio, u.created_at, u.updated_at, f.created_at
		FROM friendships f
		JOIN users u ON u.id = f.friend_id
		WHERE f.user_id = ?
		ORDER BY u.username
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list friends: %w", err)
	}
	defer rows.Close()

	var friends []model.Friend
	for rows.Next() {
		var friendSince int64
		profile, err := scanProfileRowsWithExtraTime(rows, &friendSince)
		if err != nil {
			return nil, err
		}
		friends = append(friends, model.Friend{
			User:        profile,
			FriendSince: time.Unix(friendSince, 0).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return friends, nil
}

func (s *SQLiteUserStore) GetOrCreateDirectConversation(ctx context.Context, userAID, userBID, createdBy, newConversationID string, now time.Time) (model.ConversationView, error) {
	firstID, secondID := orderedPair(userAID, userBID)
	if existingID, ok, err := s.findDirectConversationID(ctx, firstID, secondID); err != nil {
		return model.ConversationView{}, err
	} else if ok {
		return s.FindConversationByID(ctx, existingID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ConversationView{}, err
	}
	defer tx.Rollback()

	nowUnix := now.UTC().Unix()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversations (id, type, title, created_by, created_at, updated_at)
		VALUES (?, ?, '', ?, ?, ?)
	`, newConversationID, model.ConversationDirect, createdBy, nowUnix, nowUnix); err != nil {
		return model.ConversationView{}, fmt.Errorf("create conversation: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at)
		VALUES (?, ?, 'member', ?), (?, ?, 'member', ?)
	`, newConversationID, userAID, nowUnix, newConversationID, userBID, nowUnix); err != nil {
		return model.ConversationView{}, fmt.Errorf("create conversation members: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO direct_conversations (user_a_id, user_b_id, conversation_id)
		VALUES (?, ?, ?)
	`, firstID, secondID, newConversationID); err != nil {
		if isUniqueConstraint(err) {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return model.ConversationView{}, rollbackErr
			}
			existingID, ok, findErr := s.findDirectConversationID(ctx, firstID, secondID)
			if findErr != nil {
				return model.ConversationView{}, findErr
			}
			if ok {
				return s.FindConversationByID(ctx, existingID)
			}
		}
		return model.ConversationView{}, fmt.Errorf("create direct conversation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return model.ConversationView{}, err
	}

	return s.FindConversationByID(ctx, newConversationID)
}

func (s *SQLiteUserStore) CreateGroupConversation(ctx context.Context, creatorID, title string, memberIDs []string, newConversationID string, now time.Time) (model.ConversationView, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ConversationView{}, err
	}
	defer tx.Rollback()

	nowUnix := now.UTC().Unix()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversations (id, type, title, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, newConversationID, model.ConversationGroup, title, creatorID, nowUnix, nowUnix); err != nil {
		if isForeignKeyConstraint(err) {
			return model.ConversationView{}, ErrUserNotFound
		}
		return model.ConversationView{}, fmt.Errorf("create group conversation: %w", err)
	}

	memberPlaceholders := make([]string, 0, len(memberIDs)+1)
	memberArgs := make([]any, 0, (len(memberIDs)+1)*4)
	for i, memberID := range append([]string{creatorID}, memberIDs...) {
		role := "member"
		if i == 0 {
			role = "owner"
		}
		memberPlaceholders = append(memberPlaceholders, "(?, ?, ?, ?)")
		memberArgs = append(memberArgs, newConversationID, memberID, role, nowUnix)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO conversation_members (conversation_id, user_id, role, joined_at)
		VALUES `+strings.Join(memberPlaceholders, ",")+`
	`, memberArgs...); err != nil {
		if isForeignKeyConstraint(err) {
			return model.ConversationView{}, ErrUserNotFound
		}
		return model.ConversationView{}, fmt.Errorf("create group conversation members: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return model.ConversationView{}, err
	}

	return s.FindConversationByID(ctx, newConversationID)
}

func (s *SQLiteUserStore) FindConversationByID(ctx context.Context, conversationID string) (model.ConversationView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.type, c.title, c.created_by, c.created_at, c.updated_at,
		       u.id, u.username, u.display_name, u.avatar_url, u.bio, u.created_at, u.updated_at
		FROM conversations c
		JOIN conversation_members cm ON cm.conversation_id = c.id
		JOIN users u ON u.id = cm.user_id
		WHERE c.id = ?
		ORDER BY u.username
	`, conversationID)
	if err != nil {
		return model.ConversationView{}, fmt.Errorf("find conversation: %w", err)
	}
	defer rows.Close()

	var conversation model.ConversationView
	for rows.Next() {
		var (
			conversationCreatedAt int64
			conversationUpdatedAt int64
			profileCreatedAt      int64
			profileUpdatedAt      int64
			profile               model.Profile
		)
		if err := rows.Scan(
			&conversation.ID,
			&conversation.Type,
			&conversation.Title,
			&conversation.CreatedBy,
			&conversationCreatedAt,
			&conversationUpdatedAt,
			&profile.ID,
			&profile.Username,
			&profile.DisplayName,
			&profile.AvatarURL,
			&profile.Bio,
			&profileCreatedAt,
			&profileUpdatedAt,
		); err != nil {
			return model.ConversationView{}, fmt.Errorf("scan conversation: %w", err)
		}
		conversation.CreatedAt = time.Unix(conversationCreatedAt, 0).UTC()
		conversation.UpdatedAt = time.Unix(conversationUpdatedAt, 0).UTC()
		profile.CreatedAt = time.Unix(profileCreatedAt, 0).UTC()
		profile.UpdatedAt = time.Unix(profileUpdatedAt, 0).UTC()
		conversation.Members = append(conversation.Members, profile)
	}
	if err := rows.Err(); err != nil {
		return model.ConversationView{}, err
	}
	if conversation.ID == "" {
		return model.ConversationView{}, ErrConversationNotFound
	}
	return conversation, nil
}

func (s *SQLiteUserStore) findRawFriendRequest(ctx context.Context, id string) (model.FriendRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, requester_id, addressee_id, message, status, created_at, updated_at
		FROM friend_requests
		WHERE id = ?
	`, id)

	var (
		request   model.FriendRequest
		createdAt int64
		updatedAt int64
	)
	if err := row.Scan(
		&request.ID,
		&request.RequesterID,
		&request.AddresseeID,
		&request.Message,
		&request.Status,
		&createdAt,
		&updatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.FriendRequest{}, ErrFriendRequestNotFound
		}
		return model.FriendRequest{}, err
	}
	request.CreatedAt = time.Unix(createdAt, 0).UTC()
	request.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return request, nil
}

func (s *SQLiteUserStore) findDirectConversationID(ctx context.Context, userAID, userBID string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT conversation_id
		FROM direct_conversations
		WHERE user_a_id = ? AND user_b_id = ?
	`, userAID, userBID)

	var conversationID string
	switch err := row.Scan(&conversationID); {
	case err == nil:
		return conversationID, true, nil
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	default:
		return "", false, err
	}
}

func friendRequestViewSQL() string {
	return `
		SELECT fr.id, fr.message, fr.status, fr.created_at, fr.updated_at,
		       requester.id, requester.username, requester.display_name, requester.avatar_url, requester.bio, requester.created_at, requester.updated_at,
		       addressee.id, addressee.username, addressee.display_name, addressee.avatar_url, addressee.bio, addressee.created_at, addressee.updated_at
		FROM friend_requests fr
		JOIN users requester ON requester.id = fr.requester_id
		JOIN users addressee ON addressee.id = fr.addressee_id
	`
}

func scanFriendRequestView(row *sql.Row) (model.FriendRequestView, error) {
	var request model.FriendRequestView
	if err := scanFriendRequestViewScanner(row, &request); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.FriendRequestView{}, ErrFriendRequestNotFound
		}
		return model.FriendRequestView{}, err
	}
	return request, nil
}

func scanFriendRequestViewRows(rows *sql.Rows) (model.FriendRequestView, error) {
	var request model.FriendRequestView
	if err := scanFriendRequestViewScanner(rows, &request); err != nil {
		return model.FriendRequestView{}, err
	}
	return request, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanFriendRequestViewScanner(row scanner, request *model.FriendRequestView) error {
	var (
		requestCreatedAt int64
		requestUpdatedAt int64
		requesterCreated int64
		requesterUpdated int64
		addresseeCreated int64
		addresseeUpdated int64
	)
	if err := row.Scan(
		&request.ID,
		&request.Message,
		&request.Status,
		&requestCreatedAt,
		&requestUpdatedAt,
		&request.Requester.ID,
		&request.Requester.Username,
		&request.Requester.DisplayName,
		&request.Requester.AvatarURL,
		&request.Requester.Bio,
		&requesterCreated,
		&requesterUpdated,
		&request.Addressee.ID,
		&request.Addressee.Username,
		&request.Addressee.DisplayName,
		&request.Addressee.AvatarURL,
		&request.Addressee.Bio,
		&addresseeCreated,
		&addresseeUpdated,
	); err != nil {
		return err
	}
	request.CreatedAt = time.Unix(requestCreatedAt, 0).UTC()
	request.UpdatedAt = time.Unix(requestUpdatedAt, 0).UTC()
	request.Requester.CreatedAt = time.Unix(requesterCreated, 0).UTC()
	request.Requester.UpdatedAt = time.Unix(requesterUpdated, 0).UTC()
	request.Addressee.CreatedAt = time.Unix(addresseeCreated, 0).UTC()
	request.Addressee.UpdatedAt = time.Unix(addresseeUpdated, 0).UTC()
	return nil
}

func scanProfileRows(rows *sql.Rows) (model.Profile, error) {
	var (
		profile   model.Profile
		createdAt int64
		updatedAt int64
	)
	if err := rows.Scan(
		&profile.ID,
		&profile.Username,
		&profile.DisplayName,
		&profile.AvatarURL,
		&profile.Bio,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.Profile{}, fmt.Errorf("scan profile: %w", err)
	}
	profile.CreatedAt = time.Unix(createdAt, 0).UTC()
	profile.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return profile, nil
}

func scanProfileRowsWithExtraTime(rows *sql.Rows, extra *int64) (model.Profile, error) {
	var (
		profile   model.Profile
		createdAt int64
		updatedAt int64
	)
	if err := rows.Scan(
		&profile.ID,
		&profile.Username,
		&profile.DisplayName,
		&profile.AvatarURL,
		&profile.Bio,
		&createdAt,
		&updatedAt,
		extra,
	); err != nil {
		return model.Profile{}, fmt.Errorf("scan friend profile: %w", err)
	}
	profile.CreatedAt = time.Unix(createdAt, 0).UTC()
	profile.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return profile, nil
}

func orderedPair(firstID, secondID string) (string, string) {
	if firstID < secondID {
		return firstID, secondID
	}
	return secondID, firstID
}

func isForeignKeyConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "foreign key")
}
