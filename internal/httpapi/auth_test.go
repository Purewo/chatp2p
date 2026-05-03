package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatp2p/internal/auth"
	"chatp2p/internal/service"
	"chatp2p/internal/store"
)

func TestAuthFlowRegistersLogsInAndReturnsCurrentUser(t *testing.T) {
	router := newAuthTestRouter()

	registerBody := []byte(`{"username":"Alice","password":"password123","displayName":"Alice A","avatarUrl":"https://cdn.example.com/alice.png","bio":"hello from alice"}`)
	registerReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(registerBody))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()

	router.ServeHTTP(registerRec, registerReq)

	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected register status %d, got %d: %s", http.StatusCreated, registerRec.Code, registerRec.Body.String())
	}

	var registerSession service.AuthSession
	if err := json.NewDecoder(registerRec.Body).Decode(&registerSession); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	if registerSession.AccessToken == "" {
		t.Fatal("expected register response token")
	}
	if registerSession.User.Username != "alice" {
		t.Fatalf("expected normalized username alice, got %q", registerSession.User.Username)
	}
	if registerSession.User.AvatarURL != "https://cdn.example.com/alice.png" || registerSession.User.Bio != "hello from alice" {
		t.Fatalf("unexpected register profile fields: %+v", registerSession.User)
	}

	loginBody := []byte(`{"username":"alice","password":"password123"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()

	router.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login status %d, got %d: %s", http.StatusOK, loginRec.Code, loginRec.Body.String())
	}

	var loginSession service.AuthSession
	if err := json.NewDecoder(loginRec.Body).Decode(&loginSession); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginSession.AccessToken == "" {
		t.Fatal("expected login response token")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+loginSession.AccessToken)
	meRec := httptest.NewRecorder()

	router.ServeHTTP(meRec, meReq)

	if meRec.Code != http.StatusOK {
		t.Fatalf("expected me status %d, got %d: %s", http.StatusOK, meRec.Code, meRec.Body.String())
	}

	var profile struct {
		ID          string `json:"id"`
		Username    string `json:"username"`
		DisplayName string `json:"displayName"`
		AvatarURL   string `json:"avatarUrl"`
		Bio         string `json:"bio"`
	}
	if err := json.NewDecoder(meRec.Body).Decode(&profile); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if profile.ID == "" || profile.Username != "alice" || profile.DisplayName != "Alice A" || profile.AvatarURL != "https://cdn.example.com/alice.png" || profile.Bio != "hello from alice" {
		t.Fatalf("unexpected profile: %+v", profile)
	}
}

func TestRegisterRejectsDuplicateUsername(t *testing.T) {
	router := newAuthTestRouter()
	body := []byte(`{"username":"alice","password":"password123"}`)

	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first register status %d, got %d", http.StatusCreated, firstRec.Code)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, secondReq)

	if secondRec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate status %d, got %d", http.StatusConflict, secondRec.Code)
	}
}

func TestUpdateCurrentUserProfileChangesDisplayNameAvatarAndBio(t *testing.T) {
	router := newAuthTestRouter()
	session := registerTestUser(t, router, "alice")

	updateBody := []byte(`{"displayName":"Alice Updated","avatarUrl":"/avatars/alice.png","bio":"ready to chat"}`)
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	updateRec := httptest.NewRecorder()

	router.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update status %d, got %d: %s", http.StatusOK, updateRec.Code, updateRec.Body.String())
	}

	var profile struct {
		DisplayName string `json:"displayName"`
		AvatarURL   string `json:"avatarUrl"`
		Bio         string `json:"bio"`
	}
	if err := json.NewDecoder(updateRec.Body).Decode(&profile); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if profile.DisplayName != "Alice Updated" || profile.AvatarURL != "/avatars/alice.png" || profile.Bio != "ready to chat" {
		t.Fatalf("unexpected updated profile: %+v", profile)
	}
}

func TestUpdateCurrentUserProfileRejectsInvalidAvatarURL(t *testing.T) {
	router := newAuthTestRouter()
	session := registerTestUser(t, router, "alice")

	updateBody := []byte(`{"avatarUrl":"ftp://example.com/alice.png"}`)
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", bytes.NewReader(updateBody))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	updateRec := httptest.NewRecorder()

	router.ServeHTTP(updateRec, updateReq)

	if updateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected update status %d, got %d: %s", http.StatusBadRequest, updateRec.Code, updateRec.Body.String())
	}
}

func registerTestUser(t *testing.T, router http.Handler, username string) service.AuthSession {
	t.Helper()

	body := []byte(`{"username":"` + username + `","password":"password123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var session service.AuthSession
	if err := json.NewDecoder(rec.Body).Decode(&session); err != nil {
		t.Fatalf("decode register response: %v", err)
	}
	return session
}

func newAuthTestRouter() http.Handler {
	userStore := store.NewMemoryUserStore()
	tokens := auth.NewManager("test-secret", "chatp2p-test", time.Hour)
	authService := service.NewAuthService(userStore, tokens)
	return NewRouter(RouterOptions{
		ServiceName: "chatp2p-test",
		Version:     "test",
		StartedAt:   time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC),
		Auth:        authService,
	})
}
