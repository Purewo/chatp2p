package auth

import (
	"testing"
	"time"

	"chatp2p/internal/model"
)

func TestManagerIssuesAndValidatesToken(t *testing.T) {
	now := time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC)
	user := model.User{
		ID:          "user-1",
		Username:    "alice",
		DisplayName: "Alice",
	}
	manager := NewManager("test-secret", "chatp2p-test", time.Hour)

	token, _, err := manager.Issue(user, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	claims, err := manager.Validate(token, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}

	if claims.Subject != user.ID || claims.Username != user.Username {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestManagerRejectsExpiredToken(t *testing.T) {
	now := time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC)
	user := model.User{ID: "user-1", Username: "alice"}
	manager := NewManager("test-secret", "chatp2p-test", time.Second)

	token, _, err := manager.Issue(user, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	if _, err := manager.Validate(token, now.Add(2*time.Second)); err != ErrTokenExpired {
		t.Fatalf("expected expired token error, got %v", err)
	}
}
