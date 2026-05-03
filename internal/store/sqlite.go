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

type SQLiteUserStore struct {
	db *sql.DB
}

func NewSQLiteUserStore(db *sql.DB) *SQLiteUserStore {
	return &SQLiteUserStore{db: db}
}

func (s *SQLiteUserStore) Create(ctx context.Context, user model.User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, display_name, avatar_url, bio, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.Username, user.DisplayName, user.AvatarURL, user.Bio, user.PasswordHash, user.CreatedAt.Unix(), user.UpdatedAt.Unix())
	if err != nil {
		if isUniqueConstraint(err) {
			return ErrUserExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *SQLiteUserStore) FindByID(ctx context.Context, id string) (model.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, avatar_url, bio, password_hash, created_at, updated_at
		FROM users
		WHERE id = ?
	`, id)
	return scanUser(row)
}

func (s *SQLiteUserStore) FindByUsername(ctx context.Context, username string) (model.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, display_name, avatar_url, bio, password_hash, created_at, updated_at
		FROM users
		WHERE username = ?
	`, strings.ToLower(strings.TrimSpace(username)))
	return scanUser(row)
}

func (s *SQLiteUserStore) UpdateProfile(ctx context.Context, user model.User) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET display_name = ?, avatar_url = ?, bio = ?, updated_at = ?
		WHERE id = ?
	`, user.DisplayName, user.AvatarURL, user.Bio, user.UpdatedAt.Unix(), user.ID)
	if err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read updated user count: %w", err)
	}
	if rowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

func scanUser(row *sql.Row) (model.User, error) {
	var (
		user      model.User
		createdAt int64
		updatedAt int64
	)

	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.AvatarURL,
		&user.Bio,
		&user.PasswordHash,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, ErrUserNotFound
		}
		return model.User{}, fmt.Errorf("scan user: %w", err)
	}

	user.CreatedAt = time.Unix(createdAt, 0).UTC()
	user.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return user, nil
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
