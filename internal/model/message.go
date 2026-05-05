package model

import "time"

const MessageTypeText = "text"

type Message struct {
	ID             string
	ConversationID string
	SenderID       string
	Type           string
	Body           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ReadReceipt struct {
	User   Profile   `json:"user"`
	ReadAt time.Time `json:"readAt"`
}

type MessageView struct {
	ID             string        `json:"id"`
	ConversationID string        `json:"conversationId"`
	Sender         Profile       `json:"sender"`
	Type           string        `json:"type"`
	Body           string        `json:"body"`
	ReadBy         []ReadReceipt `json:"readBy"`
	EditedAt       *time.Time    `json:"editedAt,omitempty"`
	EditedBy       *Profile      `json:"editedBy,omitempty"`
	RecalledAt     *time.Time    `json:"recalledAt,omitempty"`
	RecalledBy     *Profile      `json:"recalledBy,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`
	UpdatedAt      time.Time     `json:"updatedAt"`
}

type MessageListCursor struct {
	Valid     bool
	CreatedAt time.Time
	ID        string
}

type MessageSyncEntry struct {
	ChangeID int64
	Message  MessageView
}

type ReadThroughResult struct {
	ConversationID       string    `json:"conversationId"`
	ReadThroughMessageID string    `json:"readThroughMessageId"`
	ReadAt               time.Time `json:"readAt"`
}

type MessageReadEvent struct {
	ConversationID       string    `json:"conversationId"`
	ReadThroughMessageID string    `json:"readThroughMessageId"`
	Reader               Profile   `json:"reader"`
	ReadAt               time.Time `json:"readAt"`
}
