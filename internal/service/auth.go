package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"chatp2p/internal/auth"
	"chatp2p/internal/ids"
	"chatp2p/internal/model"
	"chatp2p/internal/store"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrConflict     = errors.New("conflict")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
)

type UserStore interface {
	Create(context.Context, model.User) error
	FindByID(context.Context, string) (model.User, error)
	FindByUsername(context.Context, string) (model.User, error)
	UpdateProfile(context.Context, model.User) error
}

type AuthService struct {
	users  UserStore
	tokens *auth.Manager
	now    func() time.Time
}

type AuthSession struct {
	AccessToken string        `json:"accessToken"`
	TokenType   string        `json:"tokenType"`
	ExpiresIn   int64         `json:"expiresIn"`
	User        model.Profile `json:"user"`
}

type Credentials struct {
	Username    string
	Password    string
	DisplayName string
	AvatarURL   string
	Bio         string
}

type ProfileUpdate struct {
	DisplayName *string
	AvatarURL   *string
	Bio         *string
}

func NewAuthService(users UserStore, tokens *auth.Manager) *AuthService {
	return &AuthService{
		users:  users,
		tokens: tokens,
		now:    time.Now,
	}
}

func (s *AuthService) Register(ctx context.Context, input Credentials) (AuthSession, error) {
	user, err := s.buildUser(input)
	if err != nil {
		return AuthSession{}, err
	}

	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, store.ErrUserExists) {
			return AuthSession{}, ErrConflict
		}
		return AuthSession{}, err
	}

	return s.issueSession(user)
}

func (s *AuthService) Login(ctx context.Context, input Credentials) (AuthSession, error) {
	username, err := normalizeUsername(input.Username)
	if err != nil {
		return AuthSession{}, ErrInvalidInput
	}

	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return AuthSession{}, ErrUnauthorized
		}
		return AuthSession{}, err
	}

	if err := auth.ComparePassword(user.PasswordHash, input.Password); err != nil {
		return AuthSession{}, ErrUnauthorized
	}

	return s.issueSession(user)
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (model.Profile, error) {
	user, err := s.CurrentUser(ctx, token)
	if err != nil {
		return model.Profile{}, err
	}

	return user.Public(), nil
}

func (s *AuthService) CurrentUser(ctx context.Context, token string) (model.User, error) {
	return s.userForToken(ctx, token)
}

func (s *AuthService) UpdateProfile(ctx context.Context, token string, input ProfileUpdate) (model.Profile, error) {
	user, err := s.userForToken(ctx, token)
	if err != nil {
		return model.Profile{}, err
	}

	changed := false
	if input.DisplayName != nil {
		displayName, err := normalizeRequiredDisplayName(*input.DisplayName)
		if err != nil {
			return model.Profile{}, ErrInvalidInput
		}
		user.DisplayName = displayName
		changed = true
	}
	if input.AvatarURL != nil {
		avatarURL, err := normalizeAvatarURL(*input.AvatarURL)
		if err != nil {
			return model.Profile{}, ErrInvalidInput
		}
		user.AvatarURL = avatarURL
		changed = true
	}
	if input.Bio != nil {
		bio, err := normalizeBio(*input.Bio)
		if err != nil {
			return model.Profile{}, ErrInvalidInput
		}
		user.Bio = bio
		changed = true
	}

	if changed {
		user.UpdatedAt = s.now().UTC()
		if err := s.users.UpdateProfile(ctx, user); err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				return model.Profile{}, ErrUnauthorized
			}
			return model.Profile{}, err
		}
	}

	return user.Public(), nil
}

func (s *AuthService) userForToken(ctx context.Context, token string) (model.User, error) {
	claims, err := s.tokens.Validate(token, s.now().UTC())
	if err != nil {
		return model.User{}, ErrUnauthorized
	}

	user, err := s.users.FindByID(ctx, claims.Subject)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return model.User{}, ErrUnauthorized
		}
		return model.User{}, err
	}

	return user, nil
}

func (s *AuthService) buildUser(input Credentials) (model.User, error) {
	username, err := normalizeUsername(input.Username)
	if err != nil {
		slog.Warn("auth validation failed", "operation", "register", "field", "username", "reason", authValidationReason(err))
		return model.User{}, ErrInvalidInput
	}
	password := input.Password
	if err := validatePassword(password); err != nil {
		slog.Warn("auth validation failed", "operation", "register", "field", "password", "reason", authValidationReason(err))
		return model.User{}, ErrInvalidInput
	}
	displayName, err := normalizeDisplayName(input.DisplayName, username)
	if err != nil {
		slog.Warn("auth validation failed", "operation", "register", "field", "displayName", "reason", authValidationReason(err))
		return model.User{}, ErrInvalidInput
	}
	avatarURL, err := normalizeAvatarURL(input.AvatarURL)
	if err != nil {
		slog.Warn("auth validation failed", "operation", "register", "field", "avatarUrl", "reason", authValidationReason(err))
		return model.User{}, ErrInvalidInput
	}
	bio, err := normalizeBio(input.Bio)
	if err != nil {
		slog.Warn("auth validation failed", "operation", "register", "field", "bio", "reason", authValidationReason(err))
		return model.User{}, ErrInvalidInput
	}

	id, err := ids.New()
	if err != nil {
		return model.User{}, err
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return model.User{}, fmt.Errorf("hash password: %w", err)
	}

	now := s.now().UTC()
	return model.User{
		ID:           id,
		Username:     username,
		DisplayName:  displayName,
		AvatarURL:    avatarURL,
		Bio:          bio,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *AuthService) issueSession(user model.User) (AuthSession, error) {
	token, _, err := s.tokens.Issue(user, s.now().UTC())
	if err != nil {
		return AuthSession{}, err
	}

	return AuthSession{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(s.tokens.TTL().Seconds()),
		User:        user.Public(),
	}, nil
}

func normalizeUsername(input string) (string, error) {
	username := strings.ToLower(strings.TrimSpace(input))
	usernameLength := utf8.RuneCountInString(username)
	if usernameLength < 3 || usernameLength > 32 {
		return "", validationError("length_must_be_3_to_32")
	}

	for _, r := range username {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' {
			continue
		}
		return "", validationError("allowed_characters_are_letters_digits_underscore_dash_dot")
	}

	return username, nil
}

func normalizeDisplayName(input, fallback string) (string, error) {
	displayName := strings.TrimSpace(input)
	if displayName == "" {
		displayName = fallback
	}
	if utf8.RuneCountInString(displayName) > 32 {
		return "", validationError("length_must_be_at_most_32")
	}
	return displayName, nil
}

func normalizeRequiredDisplayName(input string) (string, error) {
	displayName := strings.TrimSpace(input)
	if displayName == "" || utf8.RuneCountInString(displayName) > 32 {
		return "", validationError("length_must_be_1_to_32")
	}
	return displayName, nil
}

func normalizeAvatarURL(input string) (string, error) {
	avatarURL := strings.TrimSpace(input)
	if avatarURL == "" {
		return "", nil
	}
	if len(avatarURL) > 2048 {
		return "", validationError("length_must_be_at_most_2048")
	}
	if strings.HasPrefix(avatarURL, "/") {
		return avatarURL, nil
	}

	parsed, err := url.Parse(avatarURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", validationError("must_be_absolute_http_url_or_app_relative_path")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", validationError("scheme_must_be_http_or_https")
	}
	return avatarURL, nil
}

func normalizeBio(input string) (string, error) {
	bio := strings.TrimSpace(input)
	if utf8.RuneCountInString(bio) > 160 {
		return "", validationError("length_must_be_at_most_160")
	}
	return bio, nil
}

func validatePassword(password string) error {
	if len(password) < 8 || len(password) > 72 {
		return validationError("length_must_be_8_to_72_bytes")
	}
	return nil
}

type validationError string

func (e validationError) Error() string {
	return string(e)
}

func authValidationReason(err error) string {
	var validationErr validationError
	if errors.As(err, &validationErr) {
		return validationErr.Error()
	}
	return "invalid"
}
