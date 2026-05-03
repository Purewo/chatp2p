package model

import "time"

const (
	FriendRequestPending  = "pending"
	FriendRequestAccepted = "accepted"
	FriendRequestDeclined = "declined"
	ConversationDirect    = "direct"
	ConversationGroup     = "group"
)

type FriendRequest struct {
	ID          string
	RequesterID string
	AddresseeID string
	Message     string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type FriendRequestView struct {
	ID        string    `json:"id"`
	Requester Profile   `json:"requester"`
	Addressee Profile   `json:"addressee"`
	Message   string    `json:"message"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Friend struct {
	User        Profile   `json:"user"`
	FriendSince time.Time `json:"friendSince"`
}

type Conversation struct {
	ID        string
	Type      string
	Title     string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ConversationView struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	CreatedBy string    `json:"createdBy"`
	Members   []Profile `json:"members"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ConversationSummary struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	Title       string       `json:"title"`
	CreatedBy   string       `json:"createdBy"`
	Members     []Profile    `json:"members"`
	LastMessage *MessageView `json:"lastMessage"`
	UnreadCount int          `json:"unreadCount"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}
