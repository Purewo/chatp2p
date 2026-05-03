package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"chatp2p/internal/auth"
	"chatp2p/internal/realtime"
	"chatp2p/internal/service"
	"chatp2p/internal/storage"
	"chatp2p/internal/store"
)

func TestSocialFlowCreatesFriendshipAndDirectConversation(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")

	searchReq := httptest.NewRequest(http.MethodGet, "/api/v1/users?query=bob", nil)
	searchReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	searchRec := httptest.NewRecorder()
	router.ServeHTTP(searchRec, searchReq)
	if searchRec.Code != http.StatusOK {
		t.Fatalf("expected search status %d, got %d: %s", http.StatusOK, searchRec.Code, searchRec.Body.String())
	}

	var searchResp struct {
		Items []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"items"`
	}
	if err := json.NewDecoder(searchRec.Body).Decode(&searchResp); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(searchResp.Items) != 1 || searchResp.Items[0].Username != "bob" {
		t.Fatalf("unexpected search response: %+v", searchResp.Items)
	}

	requestBody := []byte(`{"targetUserId":"` + bob.User.ID + `","message":"hi bob"}`)
	requestReq := httptest.NewRequest(http.MethodPost, "/api/v1/friend-requests", bytes.NewReader(requestBody))
	requestReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	requestReq.Header.Set("Content-Type", "application/json")
	requestRec := httptest.NewRecorder()
	router.ServeHTTP(requestRec, requestReq)
	if requestRec.Code != http.StatusCreated {
		t.Fatalf("expected friend request status %d, got %d: %s", http.StatusCreated, requestRec.Code, requestRec.Body.String())
	}

	var friendRequest struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(requestRec.Body).Decode(&friendRequest); err != nil {
		t.Fatalf("decode friend request: %v", err)
	}

	incomingReq := httptest.NewRequest(http.MethodGet, "/api/v1/friend-requests?box=incoming&status=pending", nil)
	incomingReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	incomingRec := httptest.NewRecorder()
	router.ServeHTTP(incomingRec, incomingReq)
	if incomingRec.Code != http.StatusOK {
		t.Fatalf("expected incoming status %d, got %d: %s", http.StatusOK, incomingRec.Code, incomingRec.Body.String())
	}

	var incoming struct {
		Items []struct {
			ID      string `json:"id"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"items"`
	}
	if err := json.NewDecoder(incomingRec.Body).Decode(&incoming); err != nil {
		t.Fatalf("decode incoming response: %v", err)
	}
	if len(incoming.Items) != 1 || incoming.Items[0].ID != friendRequest.ID || incoming.Items[0].Status != "pending" {
		t.Fatalf("unexpected incoming requests: %+v", incoming.Items)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/friend-requests/"+friendRequest.ID+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept status %d, got %d: %s", http.StatusOK, acceptRec.Code, acceptRec.Body.String())
	}

	var acceptResp struct {
		Request struct {
			Status string `json:"status"`
		} `json:"request"`
		Conversation struct {
			ID      string `json:"id"`
			Members []struct {
				Username string `json:"username"`
			} `json:"members"`
		} `json:"conversation"`
	}
	if err := json.NewDecoder(acceptRec.Body).Decode(&acceptResp); err != nil {
		t.Fatalf("decode accept response: %v", err)
	}
	if acceptResp.Request.Status != "accepted" || len(acceptResp.Conversation.Members) != 2 {
		t.Fatalf("unexpected accept response: %+v", acceptResp)
	}

	friendsReq := httptest.NewRequest(http.MethodGet, "/api/v1/friends", nil)
	friendsReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	friendsRec := httptest.NewRecorder()
	router.ServeHTTP(friendsRec, friendsReq)
	if friendsRec.Code != http.StatusOK {
		t.Fatalf("expected friends status %d, got %d: %s", http.StatusOK, friendsRec.Code, friendsRec.Body.String())
	}

	var friends struct {
		Items []struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"items"`
	}
	if err := json.NewDecoder(friendsRec.Body).Decode(&friends); err != nil {
		t.Fatalf("decode friends response: %v", err)
	}
	if len(friends.Items) != 1 || friends.Items[0].User.Username != "bob" {
		t.Fatalf("unexpected friends: %+v", friends.Items)
	}

	directBody := []byte(`{"targetUserId":"` + bob.User.ID + `"}`)
	directReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/direct", bytes.NewReader(directBody))
	directReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	directReq.Header.Set("Content-Type", "application/json")
	directRec := httptest.NewRecorder()
	router.ServeHTTP(directRec, directReq)
	if directRec.Code != http.StatusOK {
		t.Fatalf("expected direct conversation status %d, got %d: %s", http.StatusOK, directRec.Code, directRec.Body.String())
	}

	var direct struct {
		ID      string `json:"id"`
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if err := json.NewDecoder(directRec.Body).Decode(&direct); err != nil {
		t.Fatalf("decode direct response: %v", err)
	}
	if direct.ID == "" || len(direct.Members) != 2 {
		t.Fatalf("unexpected direct conversation: %+v", direct)
	}
}

func TestCreateGroupConversationUsesMembershipForMessages(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")
	carol := registerSocialUser(t, router, "carol", "Carol")
	createAcceptedFriendship(t, router, alice, bob)
	createAcceptedFriendship(t, router, alice, carol)

	groupBody := []byte(`{"title":"Weekend Plans","memberIds":["` + bob.User.ID + `","` + carol.User.ID + `"]}`)
	groupReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/group", bytes.NewReader(groupBody))
	groupReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	groupReq.Header.Set("Content-Type", "application/json")
	groupRec := httptest.NewRecorder()
	router.ServeHTTP(groupRec, groupReq)
	if groupRec.Code != http.StatusCreated {
		t.Fatalf("expected group create status %d, got %d: %s", http.StatusCreated, groupRec.Code, groupRec.Body.String())
	}

	var group struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Title   string `json:"title"`
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if err := json.NewDecoder(groupRec.Body).Decode(&group); err != nil {
		t.Fatalf("decode group conversation: %v", err)
	}
	if group.ID == "" || group.Type != "group" || group.Title != "Weekend Plans" || len(group.Members) != 3 {
		t.Fatalf("unexpected group conversation: %+v", group)
	}

	messageID := sendTextMessage(t, router, alice.AccessToken, group.ID, "hello group")
	messagesReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+group.ID+"/messages", nil)
	messagesReq.Header.Set("Authorization", "Bearer "+carol.AccessToken)
	messagesRec := httptest.NewRecorder()
	router.ServeHTTP(messagesRec, messagesReq)
	if messagesRec.Code != http.StatusOK {
		t.Fatalf("expected group messages status %d, got %d: %s", http.StatusOK, messagesRec.Code, messagesRec.Body.String())
	}

	var messages struct {
		Items []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"items"`
	}
	if err := json.NewDecoder(messagesRec.Body).Decode(&messages); err != nil {
		t.Fatalf("decode group messages: %v", err)
	}
	if len(messages.Items) != 1 || messages.Items[0].ID != messageID || messages.Items[0].Body != "hello group" {
		t.Fatalf("unexpected group messages: %+v", messages.Items)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected conversation list status %d, got %d: %s", http.StatusOK, listRec.Code, listRec.Body.String())
	}

	var list struct {
		Items []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			Title       string `json:"title"`
			UnreadCount int    `json:"unreadCount"`
			LastMessage *struct {
				ID   string `json:"id"`
				Body string `json:"body"`
			} `json:"lastMessage"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode conversation list: %v", err)
	}

	var foundGroup bool
	for _, item := range list.Items {
		if item.ID != group.ID {
			continue
		}
		foundGroup = true
		if item.Type != "group" || item.Title != "Weekend Plans" || item.UnreadCount != 1 {
			t.Fatalf("unexpected group summary: %+v", item)
		}
		if item.LastMessage == nil || item.LastMessage.ID != messageID || item.LastMessage.Body != "hello group" {
			t.Fatalf("unexpected group last message: %+v", item.LastMessage)
		}
	}
	if !foundGroup {
		t.Fatalf("group conversation %s not found in list: %+v", group.ID, list.Items)
	}
}

func TestCreateGroupConversationRequiresFriendMembers(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")

	groupBody := []byte(`{"title":"Private Group","memberIds":["` + bob.User.ID + `"]}`)
	groupReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/group", bytes.NewReader(groupBody))
	groupReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	groupReq.Header.Set("Content-Type", "application/json")
	groupRec := httptest.NewRecorder()
	router.ServeHTTP(groupRec, groupReq)
	if groupRec.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden group create status %d, got %d: %s", http.StatusForbidden, groupRec.Code, groupRec.Body.String())
	}
}

func TestGroupConversationManagement(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")
	carol := registerSocialUser(t, router, "carol", "Carol")
	dave := registerSocialUser(t, router, "dave", "Dave")
	createAcceptedFriendship(t, router, alice, bob)
	createAcceptedFriendship(t, router, alice, carol)
	createAcceptedFriendship(t, router, alice, dave)

	groupID := createGroupConversation(t, router, alice.AccessToken, "Weekend Plans", []string{bob.User.ID, carol.User.ID})

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+groupID, bytes.NewReader([]byte(`{"title":"Weekend Plans Updated"}`)))
	renameReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	router.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected rename status %d, got %d: %s", http.StatusOK, renameRec.Code, renameRec.Body.String())
	}

	var renamed struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(renameRec.Body).Decode(&renamed); err != nil {
		t.Fatalf("decode rename response: %v", err)
	}
	if renamed.Title != "Weekend Plans Updated" {
		t.Fatalf("unexpected renamed conversation: %+v", renamed)
	}

	addReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+groupID+"/members", bytes.NewReader([]byte(`{"memberIds":["`+dave.User.ID+`"]}`)))
	addReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	addReq.Header.Set("Content-Type", "application/json")
	addRec := httptest.NewRecorder()
	router.ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("expected add member status %d, got %d: %s", http.StatusOK, addRec.Code, addRec.Body.String())
	}

	var addResp struct {
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if err := json.NewDecoder(addRec.Body).Decode(&addResp); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if len(addResp.Members) != 4 {
		t.Fatalf("expected 4 members after invite, got %+v", addResp.Members)
	}

	removeReq := httptest.NewRequest(http.MethodDelete, "/api/v1/conversations/"+groupID+"/members/"+bob.User.ID, nil)
	removeReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	removeRec := httptest.NewRecorder()
	router.ServeHTTP(removeRec, removeReq)
	if removeRec.Code != http.StatusOK {
		t.Fatalf("expected remove member status %d, got %d: %s", http.StatusOK, removeRec.Code, removeRec.Body.String())
	}

	var removeResp struct {
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if err := json.NewDecoder(removeRec.Body).Decode(&removeResp); err != nil {
		t.Fatalf("decode remove response: %v", err)
	}
	if len(removeResp.Members) != 3 {
		t.Fatalf("expected 3 members after remove, got %+v", removeResp.Members)
	}

	leaveReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+groupID+"/leave", nil)
	leaveReq.Header.Set("Authorization", "Bearer "+carol.AccessToken)
	leaveRec := httptest.NewRecorder()
	router.ServeHTTP(leaveRec, leaveReq)
	if leaveRec.Code != http.StatusOK {
		t.Fatalf("expected leave status %d, got %d: %s", http.StatusOK, leaveRec.Code, leaveRec.Body.String())
	}

	var leaveResp struct {
		Members []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if err := json.NewDecoder(leaveRec.Body).Decode(&leaveResp); err != nil {
		t.Fatalf("decode leave response: %v", err)
	}
	if len(leaveResp.Members) != 2 {
		t.Fatalf("expected 2 members after leave, got %+v", leaveResp.Members)
	}

	ownerLeaveReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+groupID+"/leave", nil)
	ownerLeaveReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	ownerLeaveRec := httptest.NewRecorder()
	router.ServeHTTP(ownerLeaveRec, ownerLeaveReq)
	if ownerLeaveRec.Code != http.StatusForbidden {
		t.Fatalf("expected owner leave forbidden, got %d: %s", ownerLeaveRec.Code, ownerLeaveRec.Body.String())
	}
}

func TestGroupConversationManagementRejectsNonOwnerChanges(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")
	createAcceptedFriendship(t, router, alice, bob)

	groupID := createGroupConversation(t, router, alice.AccessToken, "Weekend Plans", []string{bob.User.ID})

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+groupID, bytes.NewReader([]byte(`{"title":"Hacked"}`)))
	renameReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	router.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-owner rename forbidden, got %d: %s", renameRec.Code, renameRec.Body.String())
	}
}

func TestGroupOwnerTransferAllowsFormerOwnerToLeave(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")
	createAcceptedFriendship(t, router, alice, bob)

	groupID := createGroupConversation(t, router, alice.AccessToken, "Weekend Plans", []string{bob.User.ID})

	transferReq := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+groupID+"/owner", bytes.NewReader([]byte(`{"targetUserId":"`+bob.User.ID+`"}`)))
	transferReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	transferReq.Header.Set("Content-Type", "application/json")
	transferRec := httptest.NewRecorder()
	router.ServeHTTP(transferRec, transferReq)
	if transferRec.Code != http.StatusOK {
		t.Fatalf("expected transfer status %d, got %d: %s", http.StatusOK, transferRec.Code, transferRec.Body.String())
	}

	var transferResp struct {
		CreatedBy string `json:"createdBy"`
	}
	if err := json.NewDecoder(transferRec.Body).Decode(&transferResp); err != nil {
		t.Fatalf("decode transfer response: %v", err)
	}
	if transferResp.CreatedBy != bob.User.ID {
		t.Fatalf("expected new owner %s, got %+v", bob.User.ID, transferResp)
	}

	leaveReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+groupID+"/leave", nil)
	leaveReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	leaveRec := httptest.NewRecorder()
	router.ServeHTTP(leaveRec, leaveReq)
	if leaveRec.Code != http.StatusOK {
		t.Fatalf("expected former owner leave status %d, got %d: %s", http.StatusOK, leaveRec.Code, leaveRec.Body.String())
	}

	renameReq := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+groupID, bytes.NewReader([]byte(`{"title":"Bob Owns It"}`)))
	renameReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	router.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected new owner rename status %d, got %d: %s", http.StatusOK, renameRec.Code, renameRec.Body.String())
	}
}

func TestSendFriendRequestRejectsReversePendingRequest(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")

	sendRequest(t, router, alice.AccessToken, bob.User.ID)
	rev := sendRequestResponse(t, router, bob.AccessToken, alice.User.ID)
	if rev.Code != http.StatusConflict {
		t.Fatalf("expected conflict on reverse pending request, got %d: %s", rev.Code, rev.Body)
	}
}

type socialSession struct {
	AccessToken string `json:"accessToken"`
	User        struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

type recordedResponse struct {
	Code int
	Body string
}

func newSocialTestRouter(t *testing.T) http.Handler {
	t.Helper()

	db := openSocialTestDB(t)
	userStore := store.NewSQLiteUserStore(db)
	authService := service.NewAuthService(userStore, auth.NewManager("test-secret", "chatp2p-test", time.Hour))
	socialService := service.NewSocialService(authService, userStore)
	messageService := service.NewMessageService(authService, userStore)
	realtimeHub := realtime.NewHub()
	return NewRouter(RouterOptions{
		ServiceName: "chatp2p-test",
		Version:     "test",
		StartedAt:   time.Date(2026, 5, 4, 1, 0, 0, 0, time.UTC),
		Auth:        authService,
		Social:      socialService,
		Messages:    messageService,
		Realtime:    realtimeHub,
	})
}

func registerSocialUser(t *testing.T, router http.Handler, username, displayName string) socialSession {
	t.Helper()

	body := []byte(`{"username":"` + username + `","password":"password123","displayName":"` + displayName + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var session socialSession
	if err := json.NewDecoder(rec.Body).Decode(&session); err != nil {
		t.Fatalf("decode register session: %v", err)
	}
	return session
}

func sendRequest(t *testing.T, router http.Handler, token, targetID string) {
	t.Helper()
	resp := sendRequestResponse(t, router, token, targetID)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected friend request status %d, got %d: %s", http.StatusCreated, resp.Code, resp.Body)
	}
}

func sendRequestResponse(t *testing.T, router http.Handler, token, targetID string) recordedResponse {
	t.Helper()

	body := []byte(`{"targetUserId":"` + targetID + `","message":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/friend-requests", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return recordedResponse{Code: rec.Code, Body: rec.Body.String()}
}

func createAcceptedFriendship(t *testing.T, router http.Handler, requester, addressee socialSession) {
	t.Helper()

	requestResp := sendRequestResponse(t, router, requester.AccessToken, addressee.User.ID)
	if requestResp.Code != http.StatusCreated {
		t.Fatalf("expected friend request status %d, got %d: %s", http.StatusCreated, requestResp.Code, requestResp.Body)
	}

	var friendRequest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(requestResp.Body), &friendRequest); err != nil {
		t.Fatalf("decode friend request: %v", err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/friend-requests/"+friendRequest.ID+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+addressee.AccessToken)
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept status %d, got %d: %s", http.StatusOK, acceptRec.Code, acceptRec.Body.String())
	}
}

func createGroupConversation(t *testing.T, router http.Handler, token, title string, memberIDs []string) string {
	t.Helper()

	reqBody := `{"title":"` + title + `","memberIds":[`
	for i, memberID := range memberIDs {
		if i > 0 {
			reqBody += ","
		}
		reqBody += `"` + memberID + `"`
	}
	reqBody += `]}`

	groupReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/group", bytes.NewReader([]byte(reqBody)))
	groupReq.Header.Set("Authorization", "Bearer "+token)
	groupReq.Header.Set("Content-Type", "application/json")
	groupRec := httptest.NewRecorder()
	router.ServeHTTP(groupRec, groupReq)
	if groupRec.Code != http.StatusCreated {
		t.Fatalf("expected group create status %d, got %d: %s", http.StatusCreated, groupRec.Code, groupRec.Body.String())
	}

	var group struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(groupRec.Body).Decode(&group); err != nil {
		t.Fatalf("decode group conversation: %v", err)
	}
	if group.ID == "" {
		t.Fatal("expected group id")
	}
	return group.ID
}

func transferGroupOwner(t *testing.T, router http.Handler, token, conversationID, targetUserID string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+conversationID+"/owner", bytes.NewReader([]byte(`{"targetUserId":"`+targetUserID+`"}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected transfer owner status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func openSocialTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := storage.Open(storage.Config{Driver: "sqlite", DSN: "file:" + t.TempDir() + "/chatp2p-social.db"})
	if err != nil {
		t.Fatalf("open social test db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	if err := storage.Migrate(context.Background(), db, "../../migrations"); err != nil {
		_ = db.Close()
		t.Fatalf("migrate social test db: %v", err)
	}
	return db
}
