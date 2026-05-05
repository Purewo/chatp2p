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

Use `DELETE /api/v1/friends/{userId}` to remove an accepted friend. Removing a friend deletes the relationship for both users and clears prior friend-request state between them so either user can send a new request later.

## Blocks

Use `GET /api/v1/blocks` to list users blocked by the current user.

Use `POST /api/v1/blocks/{userId}` to block a user. Blocking removes any friendship and friend-request state between the two users, hides both users from each other's search results, and prevents new friend requests or direct conversation entry while the block exists.

Use `DELETE /api/v1/blocks/{userId}` to unblock a user. Unblocking does not recreate a friendship; either user must send a new friend request.

## Direct Conversations

Accepting a friend request also creates or returns a one-to-one conversation. Clients can also call `POST /api/v1/conversations/direct` with a friend's `targetUserId` to get the same direct conversation later. Direct conversations require an accepted friendship.

Removing or blocking a friend does not delete existing direct conversations or message history. Existing conversation members can still load prior history, but creating or re-opening a direct conversation through `POST /api/v1/conversations/direct` requires the users to become friends again and have no active block between them.
