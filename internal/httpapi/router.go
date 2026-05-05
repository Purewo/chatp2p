package httpapi

import (
	"net/http"
	"strings"
	"time"

	"chatp2p/internal/realtime"
	"chatp2p/internal/service"
)

type RouterOptions struct {
	ServiceName        string
	Version            string
	StartedAt          time.Time
	CORSAllowedOrigins []string
	Auth               *service.AuthService
	Social             *service.SocialService
	Messages           *service.MessageService
	Realtime           *realtime.Hub
}

type API struct {
	serviceName string
	version     string
	startedAt   time.Time
	auth        *service.AuthService
	social      *service.SocialService
	messages    *service.MessageService
	realtime    *realtime.Hub
}

func NewRouter(opts RouterOptions) http.Handler {
	if opts.ServiceName == "" {
		opts.ServiceName = "chatp2p"
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.StartedAt.IsZero() {
		opts.StartedAt = time.Now().UTC()
	}

	api := &API{
		serviceName: opts.ServiceName,
		version:     opts.Version,
		startedAt:   opts.StartedAt,
		auth:        opts.Auth,
		social:      opts.Social,
		messages:    opts.Messages,
		realtime:    opts.Realtime,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.handleHealth)
	mux.HandleFunc("GET /api/v1/ws", api.handleWebSocket)
	mux.HandleFunc("GET /api/v1/sync", api.handleSync)
	mux.HandleFunc("POST /api/v1/auth/register", api.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", api.handleLogin)
	mux.HandleFunc("GET /api/v1/users/me", api.handleMe)
	mux.HandleFunc("PATCH /api/v1/users/me", api.handleUpdateMe)
	mux.HandleFunc("GET /api/v1/users", api.handleSearchUsers)
	mux.HandleFunc("POST /api/v1/friend-requests", api.handleSendFriendRequest)
	mux.HandleFunc("GET /api/v1/friend-requests", api.handleListFriendRequests)
	mux.HandleFunc("POST /api/v1/friend-requests/{id}/accept", api.handleAcceptFriendRequest)
	mux.HandleFunc("POST /api/v1/friend-requests/{id}/decline", api.handleDeclineFriendRequest)
	mux.HandleFunc("GET /api/v1/friends", api.handleListFriends)
	mux.HandleFunc("DELETE /api/v1/friends/{userId}", api.handleRemoveFriend)
	mux.HandleFunc("GET /api/v1/blocks", api.handleListBlockedUsers)
	mux.HandleFunc("POST /api/v1/blocks/{userId}", api.handleBlockUser)
	mux.HandleFunc("DELETE /api/v1/blocks/{userId}", api.handleUnblockUser)
	mux.HandleFunc("GET /api/v1/conversations", api.handleListConversations)
	mux.HandleFunc("POST /api/v1/conversations/direct", api.handleCreateDirectConversation)
	mux.HandleFunc("POST /api/v1/conversations/group", api.handleCreateGroupConversation)
	mux.HandleFunc("PATCH /api/v1/conversations/{conversationId}", api.handleRenameGroupConversation)
	mux.HandleFunc("PATCH /api/v1/conversations/{conversationId}/owner", api.handleTransferGroupConversationOwner)
	mux.HandleFunc("PATCH /api/v1/conversations/{conversationId}/settings", api.handleUpdateConversationSettings)
	mux.HandleFunc("POST /api/v1/conversations/{conversationId}/members", api.handleAddGroupConversationMembers)
	mux.HandleFunc("DELETE /api/v1/conversations/{conversationId}/members/{userId}", api.handleRemoveGroupConversationMember)
	mux.HandleFunc("POST /api/v1/conversations/{conversationId}/leave", api.handleLeaveGroupConversation)
	mux.HandleFunc("POST /api/v1/conversations/{conversationId}/messages", api.handleSendMessage)
	mux.HandleFunc("GET /api/v1/conversations/{conversationId}/messages", api.handleListMessages)
	mux.HandleFunc("PATCH /api/v1/conversations/{conversationId}/messages/{messageId}", api.handleEditMessage)
	mux.HandleFunc("POST /api/v1/conversations/{conversationId}/messages/{messageId}/recall", api.handleRecallMessage)
	mux.HandleFunc("POST /api/v1/conversations/{conversationId}/read", api.handleMarkConversationRead)
	return withCORS(mux, opts.CORSAllowedOrigins)
}

func withCORS(next http.Handler, allowedOrigins []string) http.Handler {
	if len(allowedOrigins) == 0 {
		return next
	}

	allowed := make(map[string]struct{}, len(allowedOrigins))
	allowAny := false
	for _, origin := range allowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin == "" {
			continue
		}
		if origin == "*" {
			allowAny = true
			continue
		}
		allowed[origin] = struct{}{}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		allowedOrigin := ""
		switch {
		case origin == "":
		case allowAny:
			allowedOrigin = "*"
		default:
			if _, ok := allowed[origin]; ok {
				allowedOrigin = origin
			}
		}

		if allowedOrigin != "" {
			w.Header().Add("Vary", "Origin")
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
