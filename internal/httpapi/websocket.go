package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"nhooyr.io/websocket"

	"chatp2p/internal/model"
	"chatp2p/internal/realtime"
)

func (api *API) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if api.realtime == nil || api.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "realtime service is not configured")
		return
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("accessToken"))
	}
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
		return
	}

	user, err := api.auth.CurrentUser(r.Context(), token)
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}

	userProfile := user.Public()
	api.realtime.Serve(r.Context(), realtime.ServeOptions{
		UserID: user.ID,
		Conn:   conn,
		OnConnect: func(ctx context.Context, userID string, first bool) {
			if first {
				api.publishPresenceEvent(ctx, userProfile, true)
			}
		},
		OnDisconnect: func(ctx context.Context, userID string, last bool) {
			if last {
				api.publishPresenceEvent(ctx, userProfile, false)
			}
		},
		OnMessage: func(ctx context.Context, userID string, inbound realtime.InboundMessage) {
			api.handleRealtimeInbound(ctx, userID, inbound, userProfile)
		},
	})
}

type typingPayload struct {
	ConversationID string `json:"conversationId"`
}

func (api *API) handleRealtimeInbound(ctx context.Context, userID string, inbound realtime.InboundMessage, user model.Profile) {
	if api.messages == nil {
		return
	}

	switch inbound.Type {
	case eventTypingStarted, eventTypingStopped:
		var payload typingPayload
		if err := json.Unmarshal(inbound.Data, &payload); err != nil {
			return
		}
		conversation, err := api.messages.ConversationForUser(ctx, userID, payload.ConversationID)
		if err != nil {
			return
		}

		eventData := model.TypingEvent{
			ConversationID: conversation.ID,
			User:           user,
			OccurredAt:     time.Now().UTC(),
		}
		if inbound.Type == eventTypingStarted {
			eventData.State = "started"
		} else {
			eventData.State = "stopped"
		}

		api.publishConversationEventExcept(conversation, userID, inbound.Type, eventData)
	}
}
