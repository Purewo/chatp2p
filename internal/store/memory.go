package store

import (
	"context"
	"strings"
	"sync"

	"chatp2p/internal/model"
)

type MemoryUserStore struct {
	mu        sync.RWMutex
	usersByID map[string]model.User
	idsByName map[string]string
}

func NewMemoryUserStore() *MemoryUserStore {
	return &MemoryUserStore{
		usersByID: make(map[string]model.User),
		idsByName: make(map[string]string),
	}
}

func (s *MemoryUserStore) Create(_ context.Context, user model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(user.Username)
	if _, exists := s.idsByName[key]; exists {
		return ErrUserExists
	}

	s.usersByID[user.ID] = user
	s.idsByName[key] = user.ID
	return nil
}

func (s *MemoryUserStore) FindByID(_ context.Context, id string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.usersByID[id]
	if !ok {
		return model.User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *MemoryUserStore) FindByUsername(_ context.Context, username string) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id, ok := s.idsByName[strings.ToLower(username)]
	if !ok {
		return model.User{}, ErrUserNotFound
	}

	user, ok := s.usersByID[id]
	if !ok {
		return model.User{}, ErrUserNotFound
	}
	return user, nil
}

func (s *MemoryUserStore) UpdateProfile(_ context.Context, user model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.usersByID[user.ID]
	if !ok {
		return ErrUserNotFound
	}

	existing.DisplayName = user.DisplayName
	existing.AvatarURL = user.AvatarURL
	existing.Bio = user.Bio
	existing.UpdatedAt = user.UpdatedAt
	s.usersByID[user.ID] = existing
	return nil
}
