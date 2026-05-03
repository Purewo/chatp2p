package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestWebSocketReceivesMessageAndReadEvents(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)

	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bobConn := dialTestWebSocket(t, ctx, server.URL, bob.AccessToken)
	defer bobConn.Close(websocket.StatusNormalClosure, "test done")

	aliceConn := dialTestWebSocket(t, ctx, server.URL, alice.AccessToken)
	defer aliceConn.Close(websocket.StatusNormalClosure, "test done")

	messageID := sendHTTPMessage(t, router, alice.AccessToken, conversationID, "hello websocket")

	created := readEventOfType(t, ctx, bobConn, eventMessageCreated)
	if created.Type != eventMessageCreated {
		t.Fatalf("expected event %q, got %q", eventMessageCreated, created.Type)
	}
	var message struct {
		ID   string `json:"id"`
		Body string `json:"body"`
	}
	if err := json.Unmarshal(created.Data, &message); err != nil {
		t.Fatalf("decode message event data: %v", err)
	}
	if message.ID != messageID || message.Body != "hello websocket" {
		t.Fatalf("unexpected message event data: %+v", message)
	}

	senderCreated := readEventOfType(t, ctx, aliceConn, eventMessageCreated)
	if senderCreated.Type != eventMessageCreated {
		t.Fatalf("expected sender event %q, got %q", eventMessageCreated, senderCreated.Type)
	}

	editRec := editMessage(t, router, alice.AccessToken, conversationID, messageID, "hello websocket edited")
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected edit status %d, got %d: %s", http.StatusOK, editRec.Code, editRec.Body.String())
	}

	edited := readEventOfType(t, ctx, bobConn, eventMessageEdited)
	var editedData struct {
		ID       string     `json:"id"`
		Body     string     `json:"body"`
		EditedAt *time.Time `json:"editedAt"`
		EditedBy *struct {
			Username string `json:"username"`
		} `json:"editedBy"`
	}
	if err := json.Unmarshal(edited.Data, &editedData); err != nil {
		t.Fatalf("decode edit event data: %v", err)
	}
	if editedData.ID != messageID || editedData.Body != "hello websocket edited" || editedData.EditedAt == nil || editedData.EditedBy == nil || editedData.EditedBy.Username != "alice" {
		t.Fatalf("unexpected edit event data: %+v", editedData)
	}

	senderEdited := readEventOfType(t, ctx, aliceConn, eventMessageEdited)
	if senderEdited.Type != eventMessageEdited {
		t.Fatalf("expected sender edit event %q, got %q", eventMessageEdited, senderEdited.Type)
	}

	markRead(t, router, bob.AccessToken, conversationID, messageID)

	read := readEventOfType(t, ctx, aliceConn, eventMessageRead)
	if read.Type != eventMessageRead {
		t.Fatalf("expected event %q, got %q", eventMessageRead, read.Type)
	}
	var readData struct {
		ConversationID       string `json:"conversationId"`
		ReadThroughMessageID string `json:"readThroughMessageId"`
		Reader               struct {
			Username string `json:"username"`
		} `json:"reader"`
	}
	if err := json.Unmarshal(read.Data, &readData); err != nil {
		t.Fatalf("decode read event data: %v", err)
	}
	if readData.ConversationID != conversationID || readData.ReadThroughMessageID != messageID || readData.Reader.Username != "bob" {
		t.Fatalf("unexpected read event data: %+v", readData)
	}

	recallRec := recallMessage(t, router, alice.AccessToken, conversationID, messageID)
	if recallRec.Code != http.StatusOK {
		t.Fatalf("expected recall status %d, got %d: %s", http.StatusOK, recallRec.Code, recallRec.Body.String())
	}

	recalled := readEventOfType(t, ctx, bobConn, eventMessageRecalled)
	var recalledData struct {
		ID         string     `json:"id"`
		Body       string     `json:"body"`
		RecalledAt *time.Time `json:"recalledAt"`
		RecalledBy *struct {
			Username string `json:"username"`
		} `json:"recalledBy"`
	}
	if err := json.Unmarshal(recalled.Data, &recalledData); err != nil {
		t.Fatalf("decode recall event data: %v", err)
	}
	if recalledData.ID != messageID || recalledData.Body != "" || recalledData.RecalledAt == nil || recalledData.RecalledBy == nil || recalledData.RecalledBy.Username != "alice" {
		t.Fatalf("unexpected recall event data: %+v", recalledData)
	}

	senderRecalled := readEventOfType(t, ctx, aliceConn, eventMessageRecalled)
	if senderRecalled.Type != eventMessageRecalled {
		t.Fatalf("expected sender recall event %q, got %q", eventMessageRecalled, senderRecalled.Type)
	}
}

func TestWebSocketPresenceAndTypingEvents(t *testing.T) {
	router := newSocialTestRouter(t)
	alice, bob, conversationID := setupDirectConversation(t, router)

	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bobConn := dialTestWebSocket(t, ctx, server.URL, bob.AccessToken)
	defer bobConn.Close(websocket.StatusNormalClosure, "test done")

	aliceConn := dialTestWebSocket(t, ctx, server.URL, alice.AccessToken)
	defer aliceConn.Close(websocket.StatusNormalClosure, "test done")

	presence := readEventOfType(t, ctx, bobConn, eventPresenceUpdate)
	if presence.Type != eventPresenceUpdate {
		t.Fatalf("expected presence event %q, got %q", eventPresenceUpdate, presence.Type)
	}
	var presenceData struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
		Status string `json:"status"`
		Online bool   `json:"online"`
	}
	if err := json.Unmarshal(presence.Data, &presenceData); err != nil {
		t.Fatalf("decode presence event: %v", err)
	}
	if presenceData.User.Username != "alice" || presenceData.Status != "online" || !presenceData.Online {
		t.Fatalf("unexpected presence event data: %+v", presenceData)
	}

	typingReq := realtimeInbound{
		Type: eventTypingStarted,
		Data: mustJSON(t, map[string]string{"conversationId": conversationID}),
	}
	if err := wsjson.Write(ctx, aliceConn, typingReq); err != nil {
		t.Fatalf("write typing event: %v", err)
	}

	typing := readEventOfType(t, ctx, bobConn, eventTypingStarted)
	if typing.Type != eventTypingStarted {
		t.Fatalf("expected typing event %q, got %q", eventTypingStarted, typing.Type)
	}
	var typingData struct {
		ConversationID string `json:"conversationId"`
		User           struct {
			Username string `json:"username"`
		} `json:"user"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(typing.Data, &typingData); err != nil {
		t.Fatalf("decode typing event: %v", err)
	}
	if typingData.ConversationID != conversationID || typingData.User.Username != "alice" || typingData.State != "started" {
		t.Fatalf("unexpected typing event data: %+v", typingData)
	}
}

func TestWebSocketReceivesConversationUpdatedEvents(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")
	carol := registerSocialUser(t, router, "carol", "Carol")
	createAcceptedFriendship(t, router, alice, bob)
	createAcceptedFriendship(t, router, alice, carol)
	groupID := createGroupConversation(t, router, alice.AccessToken, "Weekend Plans", []string{bob.User.ID})

	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bobConn := dialTestWebSocket(t, ctx, server.URL, bob.AccessToken)
	defer bobConn.Close(websocket.StatusNormalClosure, "test done")

	carolConn := dialTestWebSocket(t, ctx, server.URL, carol.AccessToken)
	defer carolConn.Close(websocket.StatusNormalClosure, "test done")

	renameGroupConversation(t, router, alice.AccessToken, groupID, "Weekend Plans Updated")

	renamed := readConversationAction(t, ctx, bobConn, conversationActionRenamed)
	var renamedData struct {
		ConversationID string `json:"conversationId"`
		Action         string `json:"action"`
		Actor          struct {
			Username string `json:"username"`
		} `json:"actor"`
		Conversation struct {
			Title string `json:"title"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(renamed.Data, &renamedData); err != nil {
		t.Fatalf("decode renamed event: %v", err)
	}
	if renamedData.ConversationID != groupID || renamedData.Action != conversationActionRenamed || renamedData.Actor.Username != "alice" || renamedData.Conversation.Title != "Weekend Plans Updated" {
		t.Fatalf("unexpected renamed event: %+v", renamedData)
	}

	addGroupMembers(t, router, alice.AccessToken, groupID, []string{carol.User.ID})

	added := readConversationAction(t, ctx, carolConn, conversationActionMembersAdded)
	var addedData struct {
		ConversationID string `json:"conversationId"`
		Action         string `json:"action"`
		Members        []struct {
			Username string `json:"username"`
		} `json:"members"`
	}
	if err := json.Unmarshal(added.Data, &addedData); err != nil {
		t.Fatalf("decode added event: %v", err)
	}
	if addedData.ConversationID != groupID || addedData.Action != conversationActionMembersAdded || len(addedData.Members) != 1 || addedData.Members[0].Username != "carol" {
		t.Fatalf("unexpected added event: %+v", addedData)
	}

	removeGroupMember(t, router, alice.AccessToken, groupID, bob.User.ID)

	removed := readConversationAction(t, ctx, bobConn, conversationActionMemberRemoved)
	var removedData struct {
		ConversationID string `json:"conversationId"`
		Action         string `json:"action"`
		Member         struct {
			Username string `json:"username"`
		} `json:"member"`
		Conversation struct {
			Members []struct {
				Username string `json:"username"`
			} `json:"members"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(removed.Data, &removedData); err != nil {
		t.Fatalf("decode removed event: %v", err)
	}
	if removedData.ConversationID != groupID || removedData.Action != conversationActionMemberRemoved || removedData.Member.Username != "bob" {
		t.Fatalf("unexpected removed event: %+v", removedData)
	}
	for _, member := range removedData.Conversation.Members {
		if member.Username == "bob" {
			t.Fatalf("removed member still present in conversation event: %+v", removedData.Conversation.Members)
		}
	}
}

func TestWebSocketReceivesOwnerTransferEvent(t *testing.T) {
	router := newSocialTestRouter(t)
	alice := registerSocialUser(t, router, "alice", "Alice")
	bob := registerSocialUser(t, router, "bob", "Bob")
	createAcceptedFriendship(t, router, alice, bob)
	groupID := createGroupConversation(t, router, alice.AccessToken, "Weekend Plans", []string{bob.User.ID})

	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bobConn := dialTestWebSocket(t, ctx, server.URL, bob.AccessToken)
	defer bobConn.Close(websocket.StatusNormalClosure, "test done")

	transferGroupOwner(t, router, alice.AccessToken, groupID, bob.User.ID)

	transferred := readConversationAction(t, ctx, bobConn, conversationActionOwnerTransferred)
	var transferData struct {
		ConversationID string `json:"conversationId"`
		Action         string `json:"action"`
		Actor          struct {
			Username string `json:"username"`
		} `json:"actor"`
		Member struct {
			Username string `json:"username"`
		} `json:"member"`
		Conversation struct {
			CreatedBy string `json:"createdBy"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(transferred.Data, &transferData); err != nil {
		t.Fatalf("decode transfer event: %v", err)
	}
	if transferData.ConversationID != groupID || transferData.Action != conversationActionOwnerTransferred || transferData.Actor.Username != "alice" || transferData.Member.Username != "bob" || transferData.Conversation.CreatedBy != bob.User.ID {
		t.Fatalf("unexpected transfer event: %+v", transferData)
	}
}

type realtimeTestEvent struct {
	Type   string          `json:"type"`
	Data   json.RawMessage `json:"data"`
	SentAt time.Time       `json:"sentAt"`
}

type realtimeInbound struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func readEventOfType(t *testing.T, ctx context.Context, conn *websocket.Conn, eventType string) realtimeTestEvent {
	t.Helper()

	for {
		var event realtimeTestEvent
		if err := wsjson.Read(ctx, conn, &event); err != nil {
			t.Fatalf("read websocket event: %v", err)
		}
		if event.Type == eventType {
			return event
		}
	}
}

func readConversationAction(t *testing.T, ctx context.Context, conn *websocket.Conn, action string) realtimeTestEvent {
	t.Helper()

	for {
		event := readEventOfType(t, ctx, conn, eventConversationUpdated)
		var data struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			t.Fatalf("decode conversation event action: %v", err)
		}
		if data.Action == action {
			return event
		}
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return data
}

func renameGroupConversation(t *testing.T, router http.Handler, token, conversationID, title string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/conversations/"+conversationID, bytes.NewReader([]byte(`{"title":"`+title+`"}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected rename group status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func addGroupMembers(t *testing.T, router http.Handler, token, conversationID string, memberIDs []string) {
	t.Helper()

	body := `{"memberIds":[`
	for i, memberID := range memberIDs {
		if i > 0 {
			body += ","
		}
		body += `"` + memberID + `"`
	}
	body += `]}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/members", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected add group members status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func removeGroupMember(t *testing.T, router http.Handler, token, conversationID, userID string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/conversations/"+conversationID+"/members/"+userID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected remove group member status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}

func dialTestWebSocket(t *testing.T, ctx context.Context, serverURL, token string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/v1/ws?accessToken=" + token
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	return conn
}

func sendHTTPMessage(t *testing.T, router http.Handler, token, conversationID, body string) string {
	t.Helper()

	reqBody := []byte(`{"type":"text","body":"` + body + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/messages", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected send message status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var message struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&message); err != nil {
		t.Fatalf("decode send message response: %v", err)
	}
	return message.ID
}

func markRead(t *testing.T, router http.Handler, token, conversationID, messageID string) {
	t.Helper()

	reqBody := []byte(`{"messageId":"` + messageID + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+conversationID+"/read", bytes.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected mark read status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}
}
