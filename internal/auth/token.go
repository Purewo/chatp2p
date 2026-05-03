package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatp2p/internal/model"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrTokenExpired = errors.New("token expired")
)

type Claims struct {
	Issuer      string `json:"iss"`
	Subject     string `json:"sub"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
}

type Manager struct {
	secret []byte
	issuer string
	ttl    time.Duration
}

func NewManager(secret, issuer string, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{
		secret: []byte(secret),
		issuer: issuer,
		ttl:    ttl,
	}
}

func (m *Manager) TTL() time.Duration {
	return m.ttl
}

func (m *Manager) Issue(user model.User, now time.Time) (string, Claims, error) {
	now = now.UTC()
	claims := Claims{
		Issuer:      m.issuer,
		Subject:     user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		IssuedAt:    now.Unix(),
		ExpiresAt:   now.Add(m.ttl).Unix(),
	}

	token, err := m.encode(claims)
	if err != nil {
		return "", Claims{}, err
	}

	return token, claims, nil
}

func (m *Manager) Validate(token string, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrInvalidToken
	}

	header, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var tokenHeader struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(header, &tokenHeader); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if tokenHeader.Algorithm != "HS256" || tokenHeader.Type != "JWT" {
		return Claims{}, ErrInvalidToken
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig, err := m.sign(signingInput)
	if err != nil {
		return Claims{}, err
	}

	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	if !hmac.Equal(actualSig, expectedSig) {
		return Claims{}, ErrInvalidToken
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}

	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, ErrInvalidToken
	}

	if claims.Subject == "" || claims.Username == "" {
		return Claims{}, ErrInvalidToken
	}
	if m.issuer != "" && claims.Issuer != m.issuer {
		return Claims{}, ErrInvalidToken
	}

	nowUnix := now.UTC().Unix()
	if claims.ExpiresAt > 0 && nowUnix > claims.ExpiresAt {
		return Claims{}, ErrTokenExpired
	}

	return claims, nil
}

func (m *Manager) encode(claims Claims) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}

	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal header: %w", err)
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerBytes) + "." + base64.RawURLEncoding.EncodeToString(payloadBytes)
	signature, err := m.sign(signingInput)
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (m *Manager) sign(input string) ([]byte, error) {
	mac := hmac.New(sha256.New, m.secret)
	if _, err := mac.Write([]byte(input)); err != nil {
		return nil, err
	}
	return bytes.Clone(mac.Sum(nil)), nil
}
