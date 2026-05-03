package model

import "time"

type PresenceEvent struct {
	User      Profile   `json:"user"`
	Status    string    `json:"status"`
	Online    bool      `json:"online"`
	ChangedAt time.Time `json:"changedAt"`
}

type TypingEvent struct {
	ConversationID string    `json:"conversationId"`
	User           Profile   `json:"user"`
	State          string    `json:"state"`
	OccurredAt     time.Time `json:"occurredAt"`
}

type ConversationUpdatedEvent struct {
	ConversationID string           `json:"conversationId"`
	Conversation   ConversationView `json:"conversation"`
	Actor          Profile          `json:"actor"`
	Action         string           `json:"action"`
	Member         *Profile         `json:"member,omitempty"`
	Members        []Profile        `json:"members,omitempty"`
	OccurredAt     time.Time        `json:"occurredAt"`
}
