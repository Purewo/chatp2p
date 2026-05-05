package model

import "time"

type SyncSnapshot struct {
	ServerTime    time.Time             `json:"serverTime"`
	Conversations []ConversationSummary `json:"conversations"`
	Messages      []MessageView         `json:"messages"`
	NextCursor    string                `json:"nextCursor"`
	HasMore       bool                  `json:"hasMore"`
}
