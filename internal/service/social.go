package service

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"chatp2p/internal/ids"
	"chatp2p/internal/model"
	"chatp2p/internal/store"
)

type SocialStore interface {
	FindByID(context.Context, string) (model.User, error)
	SearchUsers(context.Context, string, string, int) ([]model.Profile, error)
	CreateFriendRequest(context.Context, model.FriendRequest) error
	FindFriendRequestByID(context.Context, string) (model.FriendRequestView, error)
	ListFriendRequests(context.Context, string, string, string) ([]model.FriendRequestView, error)
	HasPendingFriendRequestBetween(context.Context, string, string) (bool, error)
	AreFriends(context.Context, string, string) (bool, error)
	HasBlockBetween(context.Context, string, string) (bool, error)
	AcceptFriendRequest(context.Context, string, string, time.Time) (model.FriendRequestView, error)
	DeclineFriendRequest(context.Context, string, string, time.Time) (model.FriendRequestView, error)
	ListFriends(context.Context, string) ([]model.Friend, error)
	RemoveFriendship(context.Context, string, string) error
	ListBlockedUsers(context.Context, string) ([]model.BlockedUser, error)
	BlockUser(context.Context, string, string, time.Time) (model.BlockedUser, error)
	UnblockUser(context.Context, string, string) error
	FindConversationByID(context.Context, string) (model.ConversationView, error)
	IsConversationMember(context.Context, string, string) (bool, error)
	GetOrCreateDirectConversation(context.Context, string, string, string, string, time.Time) (model.ConversationView, error)
	CreateGroupConversation(context.Context, string, string, []string, string, time.Time) (model.ConversationView, error)
	UpdateGroupConversationTitle(context.Context, string, string, time.Time) error
	TransferGroupConversationOwner(context.Context, string, string, string, time.Time) error
	AddConversationMembers(context.Context, string, []string, time.Time) error
	RemoveConversationMember(context.Context, string, string, time.Time) error
}

type SocialService struct {
	auth  *AuthService
	store SocialStore
	now   func() time.Time
}

type FriendRequestInput struct {
	TargetUserID string
	Message      string
}

type FriendRequestListFilter struct {
	Box    string
	Status string
}

type DirectConversationInput struct {
	TargetUserID string
}

type GroupConversationInput struct {
	Title     string
	MemberIDs []string
}

type GroupConversationRenameInput struct {
	ConversationID string
	Title          string
}

type GroupConversationMembersInput struct {
	ConversationID string
	MemberIDs      []string
}

type GroupConversationMemberInput struct {
	ConversationID string
	MemberID       string
}

type GroupConversationOwnerTransferInput struct {
	ConversationID string
	TargetUserID   string
}

type FriendRequestActionResult struct {
	Request      model.FriendRequestView `json:"request"`
	Conversation model.ConversationView  `json:"conversation"`
}

func NewSocialService(authService *AuthService, socialStore SocialStore) *SocialService {
	return &SocialService{
		auth:  authService,
		store: socialStore,
		now:   time.Now,
	}
}

func (s *SocialService) SearchUsers(ctx context.Context, token, query string, limit int) ([]model.Profile, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 32 {
		return nil, ErrInvalidInput
	}

	return s.store.SearchUsers(ctx, actor.ID, query, limit)
}

func (s *SocialService) SendFriendRequest(ctx context.Context, token string, input FriendRequestInput) (model.FriendRequestView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.FriendRequestView{}, err
	}

	targetUserID := strings.TrimSpace(input.TargetUserID)
	if targetUserID == "" || targetUserID == actor.ID {
		return model.FriendRequestView{}, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, targetUserID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return model.FriendRequestView{}, ErrNotFound
		}
		return model.FriendRequestView{}, err
	}

	areFriends, err := s.store.AreFriends(ctx, actor.ID, targetUserID)
	if err != nil {
		return model.FriendRequestView{}, err
	}
	if areFriends {
		return model.FriendRequestView{}, ErrConflict
	}
	hasBlock, err := s.store.HasBlockBetween(ctx, actor.ID, targetUserID)
	if err != nil {
		return model.FriendRequestView{}, err
	}
	if hasBlock {
		return model.FriendRequestView{}, ErrForbidden
	}
	hasPendingRequest, err := s.store.HasPendingFriendRequestBetween(ctx, actor.ID, targetUserID)
	if err != nil {
		return model.FriendRequestView{}, err
	}
	if hasPendingRequest {
		return model.FriendRequestView{}, ErrConflict
	}

	message, err := normalizeFriendRequestMessage(input.Message)
	if err != nil {
		return model.FriendRequestView{}, ErrInvalidInput
	}
	requestID, err := ids.New()
	if err != nil {
		return model.FriendRequestView{}, err
	}

	now := s.now().UTC()
	request := model.FriendRequest{
		ID:          requestID,
		RequesterID: actor.ID,
		AddresseeID: targetUserID,
		Message:     message,
		Status:      model.FriendRequestPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.CreateFriendRequest(ctx, request); err != nil {
		if errors.Is(err, store.ErrFriendRequestExists) || errors.Is(err, store.ErrAlreadyFriends) {
			return model.FriendRequestView{}, ErrConflict
		}
		if errors.Is(err, store.ErrUserNotFound) {
			return model.FriendRequestView{}, ErrNotFound
		}
		return model.FriendRequestView{}, err
	}

	return s.store.FindFriendRequestByID(ctx, requestID)
}

func (s *SocialService) ListFriendRequests(ctx context.Context, token string, filter FriendRequestListFilter) ([]model.FriendRequestView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}

	box := strings.TrimSpace(filter.Box)
	if box == "" {
		box = "incoming"
	}
	if box != "incoming" && box != "outgoing" && box != "all" {
		return nil, ErrInvalidInput
	}

	status := strings.TrimSpace(filter.Status)
	if status == "" {
		status = model.FriendRequestPending
	}
	if status != model.FriendRequestPending && status != model.FriendRequestAccepted && status != model.FriendRequestDeclined {
		return nil, ErrInvalidInput
	}

	return s.store.ListFriendRequests(ctx, actor.ID, box, status)
}

func (s *SocialService) AcceptFriendRequest(ctx context.Context, token, requestID string) (FriendRequestActionResult, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return FriendRequestActionResult{}, err
	}

	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return FriendRequestActionResult{}, ErrInvalidInput
	}

	request, err := s.store.AcceptFriendRequest(ctx, requestID, actor.ID, s.now().UTC())
	if err != nil {
		return FriendRequestActionResult{}, mapSocialStoreError(err)
	}

	conversationID, err := ids.New()
	if err != nil {
		return FriendRequestActionResult{}, err
	}
	conversation, err := s.store.GetOrCreateDirectConversation(ctx, request.Requester.ID, request.Addressee.ID, actor.ID, conversationID, s.now().UTC())
	if err != nil {
		return FriendRequestActionResult{}, err
	}

	return FriendRequestActionResult{
		Request:      request,
		Conversation: conversation,
	}, nil
}

func (s *SocialService) DeclineFriendRequest(ctx context.Context, token, requestID string) (model.FriendRequestView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.FriendRequestView{}, err
	}

	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return model.FriendRequestView{}, ErrInvalidInput
	}

	request, err := s.store.DeclineFriendRequest(ctx, requestID, actor.ID, s.now().UTC())
	if err != nil {
		return model.FriendRequestView{}, mapSocialStoreError(err)
	}
	return request, nil
}

func (s *SocialService) ListFriends(ctx context.Context, token string) ([]model.Friend, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.store.ListFriends(ctx, actor.ID)
}

func (s *SocialService) ListFriendsForUser(ctx context.Context, userID string) ([]model.Friend, error) {
	return s.store.ListFriends(ctx, userID)
}

func (s *SocialService) ListBlockedUsers(ctx context.Context, token string) ([]model.BlockedUser, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return nil, err
	}
	return s.store.ListBlockedUsers(ctx, actor.ID)
}

func (s *SocialService) BlockUser(ctx context.Context, token, blockedID string) (model.BlockedUser, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.BlockedUser{}, err
	}

	blockedID = strings.TrimSpace(blockedID)
	if blockedID == "" || blockedID == actor.ID {
		return model.BlockedUser{}, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, blockedID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return model.BlockedUser{}, ErrNotFound
		}
		return model.BlockedUser{}, err
	}

	blocked, err := s.store.BlockUser(ctx, actor.ID, blockedID, s.now().UTC())
	if err != nil {
		return model.BlockedUser{}, mapSocialStoreError(err)
	}
	return blocked, nil
}

func (s *SocialService) UnblockUser(ctx context.Context, token, blockedID string) error {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return err
	}

	blockedID = strings.TrimSpace(blockedID)
	if blockedID == "" || blockedID == actor.ID {
		return ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, blockedID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return ErrNotFound
		}
		return err
	}

	if err := s.store.UnblockUser(ctx, actor.ID, blockedID); err != nil {
		return mapSocialStoreError(err)
	}
	return nil
}

func (s *SocialService) RemoveFriend(ctx context.Context, token, friendID string) error {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return err
	}

	friendID = strings.TrimSpace(friendID)
	if friendID == "" || friendID == actor.ID {
		return ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, friendID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return ErrNotFound
		}
		return err
	}

	areFriends, err := s.store.AreFriends(ctx, actor.ID, friendID)
	if err != nil {
		return err
	}
	if !areFriends {
		return ErrNotFound
	}

	if err := s.store.RemoveFriendship(ctx, actor.ID, friendID); err != nil {
		return mapSocialStoreError(err)
	}
	return nil
}

func (s *SocialService) GetOrCreateDirectConversation(ctx context.Context, token string, input DirectConversationInput) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	targetUserID := strings.TrimSpace(input.TargetUserID)
	if targetUserID == "" || targetUserID == actor.ID {
		return model.ConversationView{}, ErrInvalidInput
	}
	if _, err := s.store.FindByID(ctx, targetUserID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return model.ConversationView{}, ErrNotFound
		}
		return model.ConversationView{}, err
	}

	areFriends, err := s.store.AreFriends(ctx, actor.ID, targetUserID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if !areFriends {
		return model.ConversationView{}, ErrForbidden
	}
	hasBlock, err := s.store.HasBlockBetween(ctx, actor.ID, targetUserID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if hasBlock {
		return model.ConversationView{}, ErrForbidden
	}

	conversationID, err := ids.New()
	if err != nil {
		return model.ConversationView{}, err
	}
	return s.store.GetOrCreateDirectConversation(ctx, actor.ID, targetUserID, actor.ID, conversationID, s.now().UTC())
}

func (s *SocialService) CreateGroupConversation(ctx context.Context, token string, input GroupConversationInput) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	title := strings.TrimSpace(input.Title)
	if title == "" || utf8.RuneCountInString(title) > 64 {
		return model.ConversationView{}, ErrInvalidInput
	}

	memberIDs, err := normalizeGroupMemberIDs(input.MemberIDs, actor.ID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if len(memberIDs) == 0 {
		return model.ConversationView{}, ErrInvalidInput
	}

	for _, memberID := range memberIDs {
		if _, err := s.store.FindByID(ctx, memberID); err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				return model.ConversationView{}, ErrNotFound
			}
			return model.ConversationView{}, err
		}

		areFriends, err := s.store.AreFriends(ctx, actor.ID, memberID)
		if err != nil {
			return model.ConversationView{}, err
		}
		if !areFriends {
			return model.ConversationView{}, ErrForbidden
		}
		hasBlock, err := s.store.HasBlockBetween(ctx, actor.ID, memberID)
		if err != nil {
			return model.ConversationView{}, err
		}
		if hasBlock {
			return model.ConversationView{}, ErrForbidden
		}
	}

	conversationID, err := ids.New()
	if err != nil {
		return model.ConversationView{}, err
	}
	return s.store.CreateGroupConversation(ctx, actor.ID, title, memberIDs, conversationID, s.now().UTC())
}

func (s *SocialService) RenameGroupConversation(ctx context.Context, token string, input GroupConversationRenameInput) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		return model.ConversationView{}, ErrInvalidInput
	}

	if _, err := s.requireOwnedGroupConversation(ctx, actor.ID, conversationID); err != nil {
		return model.ConversationView{}, err
	}

	title := strings.TrimSpace(input.Title)
	if title == "" || utf8.RuneCountInString(title) > 64 {
		return model.ConversationView{}, ErrInvalidInput
	}

	if err := s.store.UpdateGroupConversationTitle(ctx, conversationID, title, s.now().UTC()); err != nil {
		return model.ConversationView{}, mapSocialStoreError(err)
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *SocialService) TransferGroupConversationOwner(ctx context.Context, token string, input GroupConversationOwnerTransferInput) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	targetUserID := strings.TrimSpace(input.TargetUserID)
	if conversationID == "" || targetUserID == "" || targetUserID == actor.ID {
		return model.ConversationView{}, ErrInvalidInput
	}

	conversation, err := s.requireOwnedGroupConversation(ctx, actor.ID, conversationID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if _, err := findConversationMember(conversation, targetUserID); err != nil {
		return model.ConversationView{}, err
	}

	if err := s.store.TransferGroupConversationOwner(ctx, conversationID, actor.ID, targetUserID, s.now().UTC()); err != nil {
		return model.ConversationView{}, mapSocialStoreError(err)
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *SocialService) AddGroupConversationMembers(ctx context.Context, token string, input GroupConversationMembersInput) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID == "" {
		return model.ConversationView{}, ErrInvalidInput
	}

	conversation, err := s.requireOwnedGroupConversation(ctx, actor.ID, conversationID)
	if err != nil {
		return model.ConversationView{}, err
	}

	memberIDs, err := normalizeGroupMemberIDs(input.MemberIDs, actor.ID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if len(memberIDs) == 0 {
		return model.ConversationView{}, ErrInvalidInput
	}

	existing := make(map[string]struct{}, len(conversation.Members))
	for _, member := range conversation.Members {
		existing[member.ID] = struct{}{}
	}

	for _, memberID := range memberIDs {
		if _, ok := existing[memberID]; ok {
			return model.ConversationView{}, ErrConflict
		}
		if _, err := s.store.FindByID(ctx, memberID); err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				return model.ConversationView{}, ErrNotFound
			}
			return model.ConversationView{}, err
		}
		areFriends, err := s.store.AreFriends(ctx, actor.ID, memberID)
		if err != nil {
			return model.ConversationView{}, err
		}
		if !areFriends {
			return model.ConversationView{}, ErrForbidden
		}
		hasBlock, err := s.store.HasBlockBetween(ctx, actor.ID, memberID)
		if err != nil {
			return model.ConversationView{}, err
		}
		if hasBlock {
			return model.ConversationView{}, ErrForbidden
		}
	}

	if err := s.store.AddConversationMembers(ctx, conversationID, memberIDs, s.now().UTC()); err != nil {
		return model.ConversationView{}, mapSocialStoreError(err)
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *SocialService) RemoveGroupConversationMember(ctx context.Context, token, conversationID, memberID string) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	conversationID = strings.TrimSpace(conversationID)
	memberID = strings.TrimSpace(memberID)
	if conversationID == "" || memberID == "" {
		return model.ConversationView{}, ErrInvalidInput
	}

	conversation, err := s.requireOwnedGroupConversation(ctx, actor.ID, conversationID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if memberID == conversation.CreatedBy {
		return model.ConversationView{}, ErrForbidden
	}

	if _, err := findConversationMember(conversation, memberID); err != nil {
		return model.ConversationView{}, err
	}

	if err := s.store.RemoveConversationMember(ctx, conversationID, memberID, s.now().UTC()); err != nil {
		return model.ConversationView{}, mapSocialStoreError(err)
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *SocialService) LeaveGroupConversation(ctx context.Context, token, conversationID string) (model.ConversationView, error) {
	actor, err := s.auth.CurrentUser(ctx, token)
	if err != nil {
		return model.ConversationView{}, err
	}

	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return model.ConversationView{}, ErrInvalidInput
	}

	conversation, err := s.requireGroupConversation(ctx, actor.ID, conversationID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if conversation.CreatedBy == actor.ID {
		return model.ConversationView{}, ErrForbidden
	}

	if err := s.store.RemoveConversationMember(ctx, conversationID, actor.ID, s.now().UTC()); err != nil {
		return model.ConversationView{}, mapSocialStoreError(err)
	}

	return s.store.FindConversationByID(ctx, conversationID)
}

func (s *SocialService) requireGroupConversation(ctx context.Context, actorID, conversationID string) (model.ConversationView, error) {
	conversation, err := s.store.FindConversationByID(ctx, conversationID)
	if err != nil {
		return model.ConversationView{}, mapSocialStoreError(err)
	}
	if conversation.Type != model.ConversationGroup {
		return model.ConversationView{}, ErrForbidden
	}

	isMember, err := s.store.IsConversationMember(ctx, conversationID, actorID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if !isMember {
		return model.ConversationView{}, ErrForbidden
	}
	return conversation, nil
}

func (s *SocialService) requireOwnedGroupConversation(ctx context.Context, actorID, conversationID string) (model.ConversationView, error) {
	conversation, err := s.requireGroupConversation(ctx, actorID, conversationID)
	if err != nil {
		return model.ConversationView{}, err
	}
	if conversation.CreatedBy != actorID {
		return model.ConversationView{}, ErrForbidden
	}
	return conversation, nil
}

func findConversationMember(conversation model.ConversationView, memberID string) (model.Profile, error) {
	for _, member := range conversation.Members {
		if member.ID == memberID {
			return member, nil
		}
	}
	return model.Profile{}, ErrNotFound
}

func normalizeFriendRequestMessage(input string) (string, error) {
	message := strings.TrimSpace(input)
	if utf8.RuneCountInString(message) > 120 {
		return "", ErrInvalidInput
	}
	return message, nil
}

func normalizeGroupMemberIDs(memberIDs []string, actorID string) ([]string, error) {
	seen := make(map[string]struct{}, len(memberIDs))
	normalized := make([]string, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		memberID = strings.TrimSpace(memberID)
		if memberID == "" || memberID == actorID {
			return nil, ErrInvalidInput
		}
		if _, exists := seen[memberID]; exists {
			continue
		}
		seen[memberID] = struct{}{}
		normalized = append(normalized, memberID)
	}
	if len(normalized) > 50 {
		return nil, ErrInvalidInput
	}
	return normalized, nil
}

func mapSocialStoreError(err error) error {
	switch {
	case errors.Is(err, store.ErrFriendRequestNotFound), errors.Is(err, store.ErrUserNotFound), errors.Is(err, store.ErrFriendshipNotFound), errors.Is(err, store.ErrUserBlockNotFound), errors.Is(err, store.ErrConversationNotFound), errors.Is(err, store.ErrConversationMemberNotFound):
		return ErrNotFound
	case errors.Is(err, store.ErrForbidden):
		return ErrForbidden
	case errors.Is(err, store.ErrInvalidFriendRequestState), errors.Is(err, store.ErrFriendRequestExists), errors.Is(err, store.ErrAlreadyFriends), errors.Is(err, store.ErrUserBlocked), errors.Is(err, store.ErrConversationMemberExists):
		return ErrConflict
	default:
		return err
	}
}
