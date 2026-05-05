package store

import "errors"

var (
	ErrUserNotFound               = errors.New("user not found")
	ErrUserExists                 = errors.New("user already exists")
	ErrFriendRequestNotFound      = errors.New("friend request not found")
	ErrFriendRequestExists        = errors.New("friend request already exists")
	ErrInvalidFriendRequestState  = errors.New("invalid friend request state")
	ErrAlreadyFriends             = errors.New("users are already friends")
	ErrFriendshipNotFound         = errors.New("friendship not found")
	ErrUserBlocked                = errors.New("user blocked")
	ErrUserBlockNotFound          = errors.New("user block not found")
	ErrForbidden                  = errors.New("forbidden")
	ErrConversationNotFound       = errors.New("conversation not found")
	ErrConversationMemberExists   = errors.New("conversation member already exists")
	ErrConversationMemberNotFound = errors.New("conversation member not found")
	ErrMessageNotFound            = errors.New("message not found")
	ErrMessageAlreadyRecalled     = errors.New("message already recalled")
)
