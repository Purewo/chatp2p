package httpapi

import (
	"context"
	"time"

	"chatp2p/internal/model"
	"chatp2p/internal/realtime"
)

const (
	eventMessageCreated      = "message.created"
	eventMessageEdited       = "message.edited"
	eventMessageRecalled     = "message.recalled"
	eventMessageRead         = "message.read"
	eventConversationUpdated = "conversation.updated"
	eventPresenceUpdate      = "presence.updated"
	eventTypingStarted       = "typing.started"
	eventTypingStopped       = "typing.stopped"
)

func (api *API) publishConversationEvent(conversation model.ConversationView, eventType string, data any) {
	if api.realtime == nil {
		return
	}

	userIDs := conversationMemberIDs(conversation)

	api.realtime.Publish(userIDs, realtime.Event{
		Type:   eventType,
		Data:   data,
		SentAt: time.Now().UTC(),
	})
}

func (api *API) publishConversationUpdatedEvent(userIDs []string, event model.ConversationUpdatedEvent) {
	if api.realtime == nil {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	api.realtime.Publish(userIDs, realtime.Event{
		Type:   eventConversationUpdated,
		Data:   event,
		SentAt: time.Now().UTC(),
	})
}

func (api *API) publishConversationEventExcept(conversation model.ConversationView, excludedUserID string, eventType string, data any) {
	if api.realtime == nil {
		return
	}

	userIDs := make([]string, 0, len(conversation.Members))
	for _, member := range conversation.Members {
		if member.ID == excludedUserID {
			continue
		}
		userIDs = append(userIDs, member.ID)
	}

	api.realtime.Publish(userIDs, realtime.Event{
		Type:   eventType,
		Data:   data,
		SentAt: time.Now().UTC(),
	})
}

func conversationMemberIDs(conversation model.ConversationView) []string {
	userIDs := make([]string, 0, len(conversation.Members))
	for _, member := range conversation.Members {
		userIDs = append(userIDs, member.ID)
	}
	return userIDs
}

func mergeUserIDs(ids ...[]string) []string {
	seen := map[string]struct{}{}
	merged := []string{}
	for _, group := range ids {
		for _, id := range group {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			merged = append(merged, id)
		}
	}
	return merged
}

func (api *API) publishPresenceEvent(ctx context.Context, user model.Profile, online bool) {
	if api.realtime == nil || api.social == nil {
		return
	}

	friends, err := api.social.ListFriendsForUser(ctx, user.ID)
	if err != nil {
		return
	}

	userIDs := make([]string, 0, len(friends))
	for _, friend := range friends {
		userIDs = append(userIDs, friend.User.ID)
	}

	status := "offline"
	if online {
		status = "online"
	}

	api.realtime.Publish(userIDs, realtime.Event{
		Type: eventPresenceUpdate,
		Data: model.PresenceEvent{
			User:      user,
			Status:    status,
			Online:    online,
			ChangedAt: time.Now().UTC(),
		},
		SentAt: time.Now().UTC(),
	})
}
