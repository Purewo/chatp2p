# Friends and Direct Conversations

Social APIs require `Authorization: Bearer <accessToken>`.

## User Search

Use `GET /api/v1/users?query=<text>` to search by username or display name. The current user is excluded from results. `limit` defaults to 20 and is capped at 50.

## Friend Requests

- `POST /api/v1/friend-requests` sends a request to `targetUserId`.
- `GET /api/v1/friend-requests?box=incoming&status=pending` lists pending requests received by the current user.
- `POST /api/v1/friend-requests/{id}/accept` accepts an incoming pending request.
- `POST /api/v1/friend-requests/{id}/decline` declines an incoming pending request.

Only the addressee can accept or decline a request. The service rejects self-requests, duplicate pending requests between the same two users, and requests between users who are already friends.

## Friends

Use `GET /api/v1/friends` to list accepted friends. Each item contains the friend's public profile and `friendSince`.

## Direct Conversations

Accepting a friend request also creates or returns a one-to-one conversation. Clients can also call `POST /api/v1/conversations/direct` with a friend's `targetUserId` to get the same direct conversation later. Direct conversations require an accepted friendship.
