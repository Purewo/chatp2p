package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMessageFlowSendsListsAndMarksRead(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)

	sendBody := []byte(`{"type":"text","body":"hello bob"}`)
	sendReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", bytes.NewReader(sendBody))
	sendReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	sendReq.Header.Set("Content-Type", "application/json")
	sendRec := httptest.NewRecorder()
	router.ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusCreated {
		t.Fatalf("expected send status %d, got %d: %s", http.StatusCreated, sendRec.Code, sendRec.Body.String())
	}

	var sent struct {
		ID             string `json:"id"`
		ConversationID string `json:"conversationId"`
		Body           string `json:"body"`
		Sender         struct {
			Username string `json:"username"`
		} `json:"sender"`
		ReadBy []any `json:"readBy"`
	}
	if err := json.NewDecoder(sendRec.Body).Decode(&sent); err != nil {
		t.Fatalf("decode send response: %v", err)
	}
	if sent.ID == "" || sent.ConversationID != conversationID || sent.Body != "hello bob" || sent.Sender.Username != "alice" {
		t.Fatalf("unexpected sent message: %+v", sent)
	}
	if len(sent.ReadBy) != 0 {
		t.Fatalf("expected no read receipts on new message, got %+v", sent.ReadBy)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages?limit=20", nil)
	listReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d: %s", http.StatusOK, listRec.Code, listRec.Body.String())
	}

	var list struct {
		Items []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != sent.ID || list.Items[0].Body != "hello bob" {
		t.Fatalf("unexpected message list: %+v", list.Items)
	}

	readBody := []byte(`{"messageId":"` + sent.ID + `"}`)
	readReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/read", bytes.NewReader(readBody))
	readReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	readReq.Header.Set("Content-Type", "application/json")
	readRec := httptest.NewRecorder()
	router.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status %d, got %d: %s", http.StatusOK, readRec.Code, readRec.Body.String())
	}

	var readResp struct {
		ConversationID       string `json:"conversationId"`
		ReadThroughMessageID string `json:"readThroughMessageId"`
	}
	if err := json.NewDecoder(readRec.Body).Decode(&readResp); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if readResp.ConversationID != conversationID || readResp.ReadThroughMessageID != sent.ID {
		t.Fatalf("unexpected read response: %+v", readResp)
	}

	receiptReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages", nil)
	receiptReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	receiptRec := httptest.NewRecorder()
	router.ServeHTTP(receiptRec, receiptReq)
	if receiptRec.Code != http.StatusOK {
		t.Fatalf("expected receipt list status %d, got %d: %s", http.StatusOK, receiptRec.Code, receiptRec.Body.String())
	}

	var receiptList struct {
		Items []struct {
			ID     string `json:"id"`
			ReadBy []struct {
				User struct {
					Username string `json:"username"`
				} `json:"user"`
			} `json:"readBy"`
		} `json:"items"`
	}
	if err := json.NewDecoder(receiptRec.Body).Decode(&receiptList); err != nil {
		t.Fatalf("decode receipt list response: %v", err)
	}
	if len(receiptList.Items) != 1 || len(receiptList.Items[0].ReadBy) != 1 || receiptList.Items[0].ReadBy[0].User.Username != "bob" {
		t.Fatalf("unexpected read receipts: %+v", receiptList.Items)
	}
}

func TestMessageRecallUpdatesMessageAndEnforcesSender(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)
	messageID := sendTextMessage(t, router, alice.AccessToken, conversationID, "recall me")

	bobRecall := recallMessage(t, router, bob.AccessToken, conversationID, messageID)
	if bobRecall.Code != http.StatusForbidden {
		t.Fatalf("expected non-sender recall status %d, got %d: %s", http.StatusForbidden, bobRecall.Code, bobRecall.Body.String())
	}

	recallRec := recallMessage(t, router, alice.AccessToken, conversationID, messageID)
	if recallRec.Code != http.StatusOK {
		t.Fatalf("expected recall status %d, got %d: %s", http.StatusOK, recallRec.Code, recallRec.Body.String())
	}

	var recalled struct {
		ID         string     `json:"id"`
		Body       string     `json:"body"`
		RecalledAt *time.Time `json:"recalledAt"`
		RecalledBy *struct {
			Username string `json:"username"`
		} `json:"recalledBy"`
	}
	if err := json.NewDecoder(recallRec.Body).Decode(&recalled); err != nil {
		t.Fatalf("decode recall response: %v", err)
	}
	if recalled.ID != messageID || recalled.Body != "" || recalled.RecalledAt == nil || recalled.RecalledBy == nil || recalled.RecalledBy.Username != "alice" {
		t.Fatalf("unexpected recalled message: %+v", recalled)
	}

	secondRecall := recallMessage(t, router, alice.AccessToken, conversationID, messageID)
	if secondRecall.Code != http.StatusConflict {
		t.Fatalf("expected repeated recall status %d, got %d: %s", http.StatusConflict, secondRecall.Code, secondRecall.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages", nil)
	listReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list after recall status %d, got %d: %s", http.StatusOK, listRec.Code, listRec.Body.String())
	}

	var list struct {
		Items []struct {
			ID         string     `json:"id"`
			Body       string     `json:"body"`
			RecalledAt *time.Time `json:"recalledAt"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list after recall: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != messageID || list.Items[0].Body != "" || list.Items[0].RecalledAt == nil {
		t.Fatalf("unexpected recalled message list: %+v", list.Items)
	}
}

func TestMessageEditUpdatesMessageAndEnforcesSender(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)
	messageID := sendTextMessage(t, router, alice.AccessToken, conversationID, "edit me")

	bobEdit := editMessage(t, router, bob.AccessToken, conversationID, messageID, "not allowed")
	if bobEdit.Code != http.StatusForbidden {
		t.Fatalf("expected non-sender edit status %d, got %d: %s", http.StatusForbidden, bobEdit.Code, bobEdit.Body.String())
	}

	editRec := editMessage(t, router, alice.AccessToken, conversationID, messageID, "edited text")
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected edit status %d, got %d: %s", http.StatusOK, editRec.Code, editRec.Body.String())
	}

	var edited struct {
		ID       string     `json:"id"`
		Body     string     `json:"body"`
		EditedAt *time.Time `json:"editedAt"`
		EditedBy *struct {
			Username string `json:"username"`
		} `json:"editedBy"`
		RecalledAt *time.Time `json:"recalledAt"`
	}
	if err := json.NewDecoder(editRec.Body).Decode(&edited); err != nil {
		t.Fatalf("decode edit response: %v", err)
	}
	if edited.ID != messageID || edited.Body != "edited text" || edited.EditedAt == nil || edited.EditedBy == nil || edited.EditedBy.Username != "alice" || edited.RecalledAt != nil {
		t.Fatalf("unexpected edited message: %+v", edited)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/"+conversationID+"/messages", nil)
	listReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	listRec := httptest.NewRecorder()
	router.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list after edit status %d, got %d: %s", http.StatusOK, listRec.Code, listRec.Body.String())
	}

	var list struct {
		Items []struct {
			ID       string     `json:"id"`
			Body     string     `json:"body"`
			EditedAt *time.Time `json:"editedAt"`
			EditedBy *struct {
				Username string `json:"username"`
			} `json:"editedBy"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list after edit: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != messageID || list.Items[0].Body != "edited text" || list.Items[0].EditedAt == nil || list.Items[0].EditedBy == nil || list.Items[0].EditedBy.Username != "alice" {
		t.Fatalf("unexpected edited message list: %+v", list.Items)
	}

	recallRec := recallMessage(t, router, alice.AccessToken, conversationID, messageID)
	if recallRec.Code != http.StatusOK {
		t.Fatalf("expected recall before edit-after-recall status %d, got %d: %s", http.StatusOK, recallRec.Code, recallRec.Body.String())
	}

	editAfterRecall := editMessage(t, router, alice.AccessToken, conversationID, messageID, "too late")
	if editAfterRecall.Code != http.StatusConflict {
		t.Fatalf("expected edit after recall status %d, got %d: %s", http.StatusConflict, editAfterRecall.Code, editAfterRecall.Body.String())
	}
}

func TestConversationListShowsLastMessageAndUnreadCount(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)

	emptyReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10", nil)
	emptyReq.Header.Set("Authorization", "Bearer "+alice.AccessToken)
	emptyRec := httptest.NewRecorder()
	router.ServeHTTP(emptyRec, emptyReq)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("expected empty conversation list status %d, got %d: %s", http.StatusOK, emptyRec.Code, emptyRec.Body.String())
	}

	var emptyList struct {
		Items []struct {
			ID          string `json:"id"`
			LastMessage *struct {
				Body string `json:"body"`
			} `json:"lastMessage"`
			UnreadCount int `json:"unreadCount"`
		} `json:"items"`
	}
	if err := json.NewDecoder(emptyRec.Body).Decode(&emptyList); err != nil {
		t.Fatalf("decode empty conversation list: %v", err)
	}
	if len(emptyList.Items) != 1 || emptyList.Items[0].ID != conversationID || emptyList.Items[0].LastMessage != nil || emptyList.Items[0].UnreadCount != 0 {
		t.Fatalf("unexpected empty conversation list: %+v", emptyList.Items)
	}

	messageID := sendTextMessage(t, router, alice.AccessToken, conversationID, "hello bob")

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
			LastMessage *struct {
				ID     string `json:"id"`
				Body   string `json:"body"`
				Sender struct {
					Username string `json:"username"`
				} `json:"sender"`
			} `json:"lastMessage"`
			UnreadCount int `json:"unreadCount"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode conversation list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != conversationID {
		t.Fatalf("unexpected conversation list: %+v", list.Items)
	}
	if list.Items[0].LastMessage == nil || list.Items[0].LastMessage.Body != "hello bob" || list.Items[0].LastMessage.Sender.Username != "alice" {
		t.Fatalf("unexpected last message: %+v", list.Items[0].LastMessage)
	}
	if list.Items[0].UnreadCount != 1 {
		t.Fatalf("expected unread count 1, got %d", list.Items[0].UnreadCount)
	}

	readBody := []byte(`{"messageId":"` + messageID + `"}`)
	readReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/read", bytes.NewReader(readBody))
	readReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	readReq.Header.Set("Content-Type", "application/json")
	readRec := httptest.NewRecorder()
	router.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status %d, got %d: %s", http.StatusOK, readRec.Code, readRec.Body.String())
	}

	readListReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10", nil)
	readListReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	readListRec := httptest.NewRecorder()
	router.ServeHTTP(readListRec, readListReq)
	if readListRec.Code != http.StatusOK {
		t.Fatalf("expected read conversation list status %d, got %d: %s", http.StatusOK, readListRec.Code, readListRec.Body.String())
	}

	var readList struct {
		Items []struct {
			UnreadCount int `json:"unreadCount"`
		} `json:"items"`
	}
	if err := json.NewDecoder(readListRec.Body).Decode(&readList); err != nil {
		t.Fatalf("decode read conversation list: %v", err)
	}
	if len(readList.Items) != 1 || readList.Items[0].UnreadCount != 0 {
		t.Fatalf("expected unread count 0 after mark read, got %+v", readList.Items)
	}
}

func TestConversationSettingsUpdatePinsArchivesAndFiltersList(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)

	carol := registerSocialUser(t, router, "carol", "Carol")
	requestResp := sendRequestResponse(t, router, bob.AccessToken, carol.User.ID)
	if requestResp.Code != http.StatusCreated {
		t.Fatalf("expected friend request to carol status %d, got %d: %s", http.StatusCreated, requestResp.Code, requestResp.Body)
	}

	var friendRequest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(requestResp.Body), &friendRequest); err != nil {
		t.Fatalf("decode carol friend request: %v", err)
	}

	acceptReq := httptest.NewRequest(http.MethodPost, "/api/v1/friend-requests/"+friendRequest.ID+"/accept", nil)
	acceptReq.Header.Set("Authorization", "Bearer "+carol.AccessToken)
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected carol accept status %d, got %d: %s", http.StatusOK, acceptRec.Code, acceptRec.Body.String())
	}

	var accepted struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.NewDecoder(acceptRec.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode carol accept response: %v", err)
	}
	secondConversationID := accepted.Conversation.ID
	if secondConversationID == "" {
		t.Fatal("expected second conversation id")
	}

	sendTextMessage(t, router, alice.AccessToken, conversationID, "make alice conversation newer")

	beforePinReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10", nil)
	beforePinReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	beforePinRec := httptest.NewRecorder()
	router.ServeHTTP(beforePinRec, beforePinReq)
	if beforePinRec.Code != http.StatusOK {
		t.Fatalf("expected conversation list before pin status %d, got %d: %s", http.StatusOK, beforePinRec.Code, beforePinRec.Body.String())
	}

	var beforePin struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(beforePinRec.Body).Decode(&beforePin); err != nil {
		t.Fatalf("decode conversation list before pin: %v", err)
	}
	if len(beforePin.Items) != 2 {
		t.Fatalf("expected two conversations before pin, got %+v", beforePin.Items)
	}

	futureMutedUntil := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Format(time.RFC3339)
	settingsRec := updateConversationSettings(t, router, bob.AccessToken, secondConversationID, []byte(`{"pinned":true,"mutedUntil":"`+futureMutedUntil+`","archived":true}`))
	if settingsRec.Code != http.StatusOK {
		t.Fatalf("expected settings update status %d, got %d: %s", http.StatusOK, settingsRec.Code, settingsRec.Body.String())
	}

	var settings struct {
		ConversationID string     `json:"conversationId"`
		PinnedAt       *time.Time `json:"pinnedAt"`
		MutedUntil     *time.Time `json:"mutedUntil"`
		ArchivedAt     *time.Time `json:"archivedAt"`
	}
	if err := json.NewDecoder(settingsRec.Body).Decode(&settings); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if settings.ConversationID != secondConversationID || settings.PinnedAt == nil || settings.MutedUntil == nil || settings.ArchivedAt == nil {
		t.Fatalf("unexpected settings response: %+v", settings)
	}

	defaultListReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10", nil)
	defaultListReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	defaultListRec := httptest.NewRecorder()
	router.ServeHTTP(defaultListRec, defaultListReq)
	if defaultListRec.Code != http.StatusOK {
		t.Fatalf("expected default list status %d, got %d: %s", http.StatusOK, defaultListRec.Code, defaultListRec.Body.String())
	}

	var defaultList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(defaultListRec.Body).Decode(&defaultList); err != nil {
		t.Fatalf("decode default conversation list: %v", err)
	}
	if len(defaultList.Items) != 1 || defaultList.Items[0].ID != conversationID {
		t.Fatalf("unexpected default list after archive: %+v", defaultList.Items)
	}

	includeArchivedReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10&includeArchived=true", nil)
	includeArchivedReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	includeArchivedRec := httptest.NewRecorder()
	router.ServeHTTP(includeArchivedRec, includeArchivedReq)
	if includeArchivedRec.Code != http.StatusOK {
		t.Fatalf("expected includeArchived list status %d, got %d: %s", http.StatusOK, includeArchivedRec.Code, includeArchivedRec.Body.String())
	}

	var archivedList struct {
		Items []struct {
			ID         string     `json:"id"`
			PinnedAt   *time.Time `json:"pinnedAt"`
			MutedUntil *time.Time `json:"mutedUntil"`
			ArchivedAt *time.Time `json:"archivedAt"`
		} `json:"items"`
	}
	if err := json.NewDecoder(includeArchivedRec.Body).Decode(&archivedList); err != nil {
		t.Fatalf("decode includeArchived conversation list: %v", err)
	}
	if len(archivedList.Items) != 2 || archivedList.Items[0].ID != secondConversationID || archivedList.Items[1].ID != conversationID {
		t.Fatalf("unexpected includeArchived order: %+v", archivedList.Items)
	}
	if archivedList.Items[0].PinnedAt == nil || archivedList.Items[0].MutedUntil == nil || archivedList.Items[0].ArchivedAt == nil {
		t.Fatalf("expected archived pinned conversation settings in list: %+v", archivedList.Items[0])
	}

	syncSince := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	syncReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync?since="+syncSince+"&limit=10", nil)
	syncReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	syncRec := httptest.NewRecorder()
	router.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected sync with archived settings status %d, got %d: %s", http.StatusOK, syncRec.Code, syncRec.Body.String())
	}

	var syncResp struct {
		Conversations []struct {
			ID         string     `json:"id"`
			PinnedAt   *time.Time `json:"pinnedAt"`
			MutedUntil *time.Time `json:"mutedUntil"`
			ArchivedAt *time.Time `json:"archivedAt"`
		} `json:"conversations"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync with archived settings: %v", err)
	}
	if len(syncResp.Conversations) != 2 || syncResp.Conversations[0].ID != secondConversationID {
		t.Fatalf("unexpected sync conversations with settings: %+v", syncResp.Conversations)
	}
	if syncResp.Conversations[0].PinnedAt == nil || syncResp.Conversations[0].MutedUntil == nil || syncResp.Conversations[0].ArchivedAt == nil {
		t.Fatalf("expected archived settings in sync conversation: %+v", syncResp.Conversations[0])
	}

	clearRec := updateConversationSettings(t, router, bob.AccessToken, secondConversationID, []byte(`{"pinned":false,"mutedUntil":"","archived":false}`))
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected settings clear status %d, got %d: %s", http.StatusOK, clearRec.Code, clearRec.Body.String())
	}

	var cleared struct {
		ConversationID string     `json:"conversationId"`
		PinnedAt       *time.Time `json:"pinnedAt"`
		MutedUntil     *time.Time `json:"mutedUntil"`
		ArchivedAt     *time.Time `json:"archivedAt"`
	}
	if err := json.NewDecoder(clearRec.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared settings response: %v", err)
	}
	if cleared.ConversationID != secondConversationID || cleared.PinnedAt != nil || cleared.MutedUntil != nil || cleared.ArchivedAt != nil {
		t.Fatalf("unexpected cleared settings response: %+v", cleared)
	}

	clearedListReq := httptest.NewRequest(http.MethodGet, "/api/v1/conversations?limit=10", nil)
	clearedListReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	clearedListRec := httptest.NewRecorder()
	router.ServeHTTP(clearedListRec, clearedListReq)
	if clearedListRec.Code != http.StatusOK {
		t.Fatalf("expected cleared list status %d, got %d: %s", http.StatusOK, clearedListRec.Code, clearedListRec.Body.String())
	}

	var clearedList struct {
		Items []struct {
			ID         string     `json:"id"`
			PinnedAt   *time.Time `json:"pinnedAt"`
			MutedUntil *time.Time `json:"mutedUntil"`
			ArchivedAt *time.Time `json:"archivedAt"`
		} `json:"items"`
	}
	if err := json.NewDecoder(clearedListRec.Body).Decode(&clearedList); err != nil {
		t.Fatalf("decode cleared conversation list: %v", err)
	}
	if len(clearedList.Items) != 2 {
		t.Fatalf("expected two conversations after clear, got %+v", clearedList.Items)
	}
	for _, item := range clearedList.Items {
		if item.ID == secondConversationID && (item.PinnedAt != nil || item.MutedUntil != nil || item.ArchivedAt != nil) {
			t.Fatalf("expected cleared settings to be nil in list: %+v", item)
		}
	}
}

func TestSyncReturnsConversationsAndMessagesSinceCursor(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)
	since := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)

	messageID := sendTextMessage(t, router, alice.AccessToken, conversationID, "sync me")

	syncReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync?since="+since+"&limit=10", nil)
	syncReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	syncRec := httptest.NewRecorder()
	router.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected sync status %d, got %d: %s", http.StatusOK, syncRec.Code, syncRec.Body.String())
	}

	var syncResp struct {
		ServerTime    string `json:"serverTime"`
		Conversations []struct {
			ID          string `json:"id"`
			UnreadCount int    `json:"unreadCount"`
			LastMessage *struct {
				ID   string `json:"id"`
				Body string `json:"body"`
			} `json:"lastMessage"`
		} `json:"conversations"`
		Messages []struct {
			ID             string `json:"id"`
			ConversationID string `json:"conversationId"`
			Body           string `json:"body"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if syncResp.ServerTime == "" {
		t.Fatal("expected serverTime")
	}
	if len(syncResp.Conversations) != 1 || syncResp.Conversations[0].ID != conversationID {
		t.Fatalf("unexpected synced conversations: %+v", syncResp.Conversations)
	}
	if syncResp.Conversations[0].UnreadCount != 1 {
		t.Fatalf("expected unread count 1, got %d", syncResp.Conversations[0].UnreadCount)
	}
	if syncResp.Conversations[0].LastMessage == nil || syncResp.Conversations[0].LastMessage.ID != messageID {
		t.Fatalf("unexpected synced last message: %+v", syncResp.Conversations[0].LastMessage)
	}
	if len(syncResp.Messages) != 1 || syncResp.Messages[0].ID != messageID || syncResp.Messages[0].Body != "sync me" {
		t.Fatalf("unexpected synced messages: %+v", syncResp.Messages)
	}

	markConversationRead(t, router, bob.AccessToken, conversationID, messageID)

	readSyncReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync?since="+since+"&limit=10", nil)
	readSyncReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	readSyncRec := httptest.NewRecorder()
	router.ServeHTTP(readSyncRec, readSyncReq)
	if readSyncRec.Code != http.StatusOK {
		t.Fatalf("expected sync after read status %d, got %d: %s", http.StatusOK, readSyncRec.Code, readSyncRec.Body.String())
	}

	var readSyncResp struct {
		Conversations []struct {
			UnreadCount int `json:"unreadCount"`
			LastMessage *struct {
				ID         string     `json:"id"`
				Body       string     `json:"body"`
				RecalledAt *time.Time `json:"recalledAt"`
			} `json:"lastMessage"`
		} `json:"conversations"`
		Messages []struct {
			ID         string     `json:"id"`
			Body       string     `json:"body"`
			RecalledAt *time.Time `json:"recalledAt"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(readSyncRec.Body).Decode(&readSyncResp); err != nil {
		t.Fatalf("decode sync after read response: %v", err)
	}
	if len(readSyncResp.Conversations) != 1 || readSyncResp.Conversations[0].UnreadCount != 0 {
		t.Fatalf("expected unread count 0 after read sync, got %+v", readSyncResp.Conversations)
	}

	editRec := editMessage(t, router, alice.AccessToken, conversationID, messageID, "sync edited")
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected edit during sync test status %d, got %d: %s", http.StatusOK, editRec.Code, editRec.Body.String())
	}

	editSyncReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync?since="+since+"&limit=10", nil)
	editSyncReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	editSyncRec := httptest.NewRecorder()
	router.ServeHTTP(editSyncRec, editSyncReq)
	if editSyncRec.Code != http.StatusOK {
		t.Fatalf("expected sync after edit status %d, got %d: %s", http.StatusOK, editSyncRec.Code, editSyncRec.Body.String())
	}

	var editSyncResp struct {
		Conversations []struct {
			LastMessage *struct {
				ID       string     `json:"id"`
				Body     string     `json:"body"`
				EditedAt *time.Time `json:"editedAt"`
				EditedBy *struct {
					Username string `json:"username"`
				} `json:"editedBy"`
			} `json:"lastMessage"`
		} `json:"conversations"`
		Messages []struct {
			ID       string     `json:"id"`
			Body     string     `json:"body"`
			EditedAt *time.Time `json:"editedAt"`
			EditedBy *struct {
				Username string `json:"username"`
			} `json:"editedBy"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(editSyncRec.Body).Decode(&editSyncResp); err != nil {
		t.Fatalf("decode sync after edit response: %v", err)
	}
	if len(editSyncResp.Conversations) != 1 || editSyncResp.Conversations[0].LastMessage == nil {
		t.Fatalf("unexpected sync conversations after edit: %+v", editSyncResp.Conversations)
	}
	editedLastMessage := editSyncResp.Conversations[0].LastMessage
	if editedLastMessage.ID != messageID || editedLastMessage.Body != "sync edited" || editedLastMessage.EditedAt == nil || editedLastMessage.EditedBy == nil || editedLastMessage.EditedBy.Username != "alice" {
		t.Fatalf("unexpected synced last message after edit: %+v", editedLastMessage)
	}
	if len(editSyncResp.Messages) != 1 || editSyncResp.Messages[0].ID != messageID || editSyncResp.Messages[0].Body != "sync edited" || editSyncResp.Messages[0].EditedAt == nil || editSyncResp.Messages[0].EditedBy == nil || editSyncResp.Messages[0].EditedBy.Username != "alice" {
		t.Fatalf("unexpected synced edited messages: %+v", editSyncResp.Messages)
	}

	recallRec := recallMessage(t, router, alice.AccessToken, conversationID, messageID)
	if recallRec.Code != http.StatusOK {
		t.Fatalf("expected recall during sync test status %d, got %d: %s", http.StatusOK, recallRec.Code, recallRec.Body.String())
	}

	recallSyncReq := httptest.NewRequest(http.MethodGet, "/api/v1/sync?since="+since+"&limit=10", nil)
	recallSyncReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	recallSyncRec := httptest.NewRecorder()
	router.ServeHTTP(recallSyncRec, recallSyncReq)
	if recallSyncRec.Code != http.StatusOK {
		t.Fatalf("expected sync after recall status %d, got %d: %s", http.StatusOK, recallSyncRec.Code, recallSyncRec.Body.String())
	}

	var recallSyncResp struct {
		Conversations []struct {
			UnreadCount int `json:"unreadCount"`
			LastMessage *struct {
				ID         string     `json:"id"`
				Body       string     `json:"body"`
				RecalledAt *time.Time `json:"recalledAt"`
			} `json:"lastMessage"`
		} `json:"conversations"`
		Messages []struct {
			ID         string     `json:"id"`
			Body       string     `json:"body"`
			RecalledAt *time.Time `json:"recalledAt"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(recallSyncRec.Body).Decode(&recallSyncResp); err != nil {
		t.Fatalf("decode sync after recall response: %v", err)
	}
	if len(recallSyncResp.Conversations) != 1 || recallSyncResp.Conversations[0].LastMessage == nil {
		t.Fatalf("unexpected sync conversations after recall: %+v", recallSyncResp.Conversations)
	}
	lastMessage := recallSyncResp.Conversations[0].LastMessage
	if lastMessage.ID != messageID || lastMessage.Body != "" || lastMessage.RecalledAt == nil {
		t.Fatalf("unexpected synced last message after recall: %+v", lastMessage)
	}
	if len(recallSyncResp.Messages) != 1 || recallSyncResp.Messages[0].ID != messageID || recallSyncResp.Messages[0].Body != "" || recallSyncResp.Messages[0].RecalledAt == nil {
		t.Fatalf("unexpected synced recalled messages: %+v", recallSyncResp.Messages)
	}
}

func setupDirectConversation(t *testing.T, router http.Handler) (socialSession, socialSession, string) {
	t.Helper()

	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")

	requestResp := sendRequestResponse(t, router, alice.AccessToken, bob.User.ID)
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
	acceptReq.Header.Set("Authorization", "Bearer "+bob.AccessToken)
	acceptRec := httptest.NewRecorder()
	router.ServeHTTP(acceptRec, acceptReq)
	if acceptRec.Code != http.StatusOK {
		t.Fatalf("expected accept status %d, got %d: %s", http.StatusOK, acceptRec.Code, acceptRec.Body.String())
	}

	var accepted struct {
		Conversation struct {
			ID string `json:"id"`
		} `json:"conversation"`
	}
	if err := json.NewDecoder(acceptRec.Body).Decode(&accepted); err != nil {
		t.Fatalf("decode accept response: %v", err)
	}
	if accepted.Conversation.ID == "" {
		t.Fatal("expected conversation id")
	}

	return alice, bob, accepted.Conversation.ID
}

func sendTextMessage(t *testing.T, router http.Handler, token, conversationID, body string) string {
	t.Helper()

	sendBody := []byte(`{"type":"text","body":"` + body + `"}`)
	sendReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", bytes.NewReader(sendBody))
	sendReq.Header.Set("Authorization", "Bearer "+token)
	sendReq.Header.Set("Content-Type", "application/json")
	sendRec := httptest.NewRecorder()
	router.ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusCreated {
		t.Fatalf("expected send status %d, got %d: %s", http.StatusCreated, sendRec.Code, sendRec.Body.String())
	}

	var sent struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(sendRec.Body).Decode(&sent); err != nil {
		t.Fatalf("decode sent message: %v", err)
	}
	if sent.ID == "" {
		t.Fatal("expected message id")
	}
	return sent.ID
}

func markConversationRead(t *testing.T, router http.Handler, token, conversationID, messageID string) {
	t.Helper()

	readBody := []byte(`{"messageId":"` + messageID + `"}`)
	readReq := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/read", bytes.NewReader(readBody))
	readReq.Header.Set("Authorization", "Bearer "+token)
	readReq.Header.Set("Content-Type", "application/json")
	readRec := httptest.NewRecorder()
	router.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected read status %d, got %d: %s", http.StatusOK, readRec.Code, readRec.Body.String())
	}
}

func recallMessage(t *testing.T, router http.Handler, token, conversationID, messageID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages/"+messageID+"/recall", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func editMessage(t *testing.T, router http.Handler, token, conversationID, messageID, body string) *httptest.ResponseRecorder {
	t.Helper()

	reqBody, err := json.Marshal(map[string]string{"body": body})
	if err != nil {
		t.Fatalf("marshal edit request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+conversationID+"/messages/"+messageID, bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func updateConversationSettings(t *testing.T, router http.Handler, token, conversationID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+conversationID+"/settings", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
