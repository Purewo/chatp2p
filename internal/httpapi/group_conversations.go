package httpapi

import (
	"context"
	"net/http"
	"time"

	"chatp2p/internal/model"
	"chatp2p/internal/service"
)

const (
	conversationActionRenamed          = "renamed"
	conversationActionOwnerTransferred = "owner_transferred"
	conversationActionMembersAdded     = "members_added"
	conversationActionMemberRemoved    = "member_removed"
	conversationActionMemberLeft       = "member_left"
)

type renameGroupConversationRequest struct {
	Title string `json:"title"`
}

type addGroupConversationMembersRequest struct {
	MemberIDs []string `json:"memberIds"`
}

type transferGroupConversationOwnerRequest struct {
	TargetUserID string `json:"targetUserId"`
}

func (api *API) handleRenameGroupConversation(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	var req renameGroupConversationRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	conversation, err := api.social.RenameGroupConversation(r.Context(), token, service.GroupConversationRenameInput{
		ConversationID: r.PathValue("conversationId"),
		Title:          req.Title,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if actor, ok := api.eventActor(r.Context(), token); ok {
		api.publishConversationUpdatedEvent(conversationMemberIDs(conversation), model.ConversationUpdatedEvent{
			ConversationID: conversation.ID,
			Conversation:   conversation,
			Actor:          actor,
			Action:         conversationActionRenamed,
			OccurredAt:     time.Now().UTC(),
		})
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (api *API) handleTransferGroupConversationOwner(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	var req transferGroupConversationOwnerRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	conversation, err := api.social.TransferGroupConversationOwner(r.Context(), token, service.GroupConversationOwnerTransferInput{
		ConversationID: r.PathValue("conversationId"),
		TargetUserID:   req.TargetUserID,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if actor, ok := api.eventActor(r.Context(), token); ok {
		var member *model.Profile
		if profile, ok := conversationProfileByID(conversation, req.TargetUserID); ok {
			member = &profile
		}
		api.publishConversationUpdatedEvent(conversationMemberIDs(conversation), model.ConversationUpdatedEvent{
			ConversationID: conversation.ID,
			Conversation:   conversation,
			Actor:          actor,
			Action:         conversationActionOwnerTransferred,
			Member:         member,
			OccurredAt:     time.Now().UTC(),
		})
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (api *API) handleAddGroupConversationMembers(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	var req addGroupConversationMembersRequest
	if err := readJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body is invalid JSON")
		return
	}

	conversation, err := api.social.AddGroupConversationMembers(r.Context(), token, service.GroupConversationMembersInput{
		ConversationID: r.PathValue("conversationId"),
		MemberIDs:      req.MemberIDs,
	})
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if actor, ok := api.eventActor(r.Context(), token); ok {
		api.publishConversationUpdatedEvent(conversationMemberIDs(conversation), model.ConversationUpdatedEvent{
			ConversationID: conversation.ID,
			Conversation:   conversation,
			Actor:          actor,
			Action:         conversationActionMembersAdded,
			Members:        conversationProfilesByID(conversation, req.MemberIDs),
			OccurredAt:     time.Now().UTC(),
		})
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (api *API) handleRemoveGroupConversationMember(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	before, hadBefore := api.conversationBeforeChange(r.Context(), token, r.PathValue("conversationId"))
	removedMember, hadRemovedMember := conversationProfileByID(before, r.PathValue("userId"))

	conversation, err := api.social.RemoveGroupConversationMember(r.Context(), token, r.PathValue("conversationId"), r.PathValue("userId"))
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if actor, ok := api.eventActor(r.Context(), token); ok {
		recipients := conversationMemberIDs(conversation)
		if hadBefore {
			recipients = mergeUserIDs(conversationMemberIDs(before), recipients)
		}
		event := model.ConversationUpdatedEvent{
			ConversationID: conversation.ID,
			Conversation:   conversation,
			Actor:          actor,
			Action:         conversationActionMemberRemoved,
			OccurredAt:     time.Now().UTC(),
		}
		if hadRemovedMember {
			event.Member = &removedMember
		}
		api.publishConversationUpdatedEvent(recipients, event)
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (api *API) handleLeaveGroupConversation(w http.ResponseWriter, r *http.Request) {
	token := api.socialToken(w, r)
	if token == "" {
		return
	}

	before, hadBefore := api.conversationBeforeChange(r.Context(), token, r.PathValue("conversationId"))

	conversation, err := api.social.LeaveGroupConversation(r.Context(), token, r.PathValue("conversationId"))
	if err != nil {
		api.writeServiceError(w, err)
		return
	}

	if actor, ok := api.eventActor(r.Context(), token); ok {
		recipients := conversationMemberIDs(conversation)
		if hadBefore {
			recipients = mergeUserIDs(conversationMemberIDs(before), recipients)
		}
		member := actor
		api.publishConversationUpdatedEvent(recipients, model.ConversationUpdatedEvent{
			ConversationID: conversation.ID,
			Conversation:   conversation,
			Actor:          actor,
			Action:         conversationActionMemberLeft,
			Member:         &member,
			OccurredAt:     time.Now().UTC(),
		})
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (api *API) eventActor(ctx context.Context, token string) (model.Profile, bool) {
	if api.auth == nil {
		return model.Profile{}, false
	}
	actor, err := api.auth.Authenticate(ctx, token)
	if err != nil {
		return model.Profile{}, false
	}
	return actor, true
}

func (api *API) conversationBeforeChange(ctx context.Context, token, conversationID string) (model.ConversationView, bool) {
	if api.messages == nil {
		return model.ConversationView{}, false
	}
	conversation, err := api.messages.Conversation(ctx, token, conversationID)
	if err != nil {
		return model.ConversationView{}, false
	}
	return conversation, true
}

func conversationProfilesByID(conversation model.ConversationView, userIDs []string) []model.Profile {
	profiles := make([]model.Profile, 0, len(userIDs))
	for _, userID := range userIDs {
		if profile, ok := conversationProfileByID(conversation, userID); ok {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

func conversationProfileByID(conversation model.ConversationView, userID string) (model.Profile, bool) {
	for _, member := range conversation.Members {
		if member.ID == userID {
			return member, true
		}
	}
	return model.Profile{}, false
}
