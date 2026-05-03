package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"chatp2p/internal/model"
	"chatp2p/internal/storage"
)

func TestSQLiteUserStoreCreatesAndFindsUser(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	defer db.Close()

	users := NewSQLiteUserStore(db)
	now := time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC)
	user := model.User{
		ID:           "8f99c7f7-5968-4f1b-a63e-55806b9fbaf9",
		Username:     "alice",
		DisplayName:  "Alice",
		AvatarURL:    "https://cdn.example.com/alice.png",
		Bio:          "hello",
		PasswordHash: "hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := users.Create(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	found, err := users.FindByUsername(ctx, "Alice")
	if err != nil {
		t.Fatalf("find user by username: %v", err)
	}

	if found.ID != user.ID || found.Username != user.Username || found.DisplayName != user.DisplayName || found.AvatarURL != user.AvatarURL || found.Bio != user.Bio {
		t.Fatalf("unexpected found user: %+v", found)
	}

	if err := users.Create(ctx, user); err != ErrUserExists {
		t.Fatalf("expected duplicate error %v, got %v", ErrUserExists, err)
	}

	user.DisplayName = "Alice Updated"
	user.AvatarURL = "/avatars/alice.png"
	user.Bio = "updated bio"
	user.UpdatedAt = now.Add(time.Minute)
	if err := users.UpdateProfile(ctx, user); err != nil {
		t.Fatalf("update user profile: %v", err)
	}

	updated, err := users.FindByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("find updated user by id: %v", err)
	}
	if updated.DisplayName != "Alice Updated" || updated.AvatarURL != "/avatars/alice.png" || updated.Bio != "updated bio" {
		t.Fatalf("unexpected updated user: %+v", updated)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := t.TempDir() + "/chatp2p-test.db"
	db, err := storage.Open(storage.Config{Driver: "sqlite", DSN: "file:" + dbPath})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := storage.Migrate(context.Background(), db, "../../migrations"); err != nil {
		db.Close()
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}
