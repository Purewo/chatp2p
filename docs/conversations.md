# Conversations

Conversation APIs are for the chat home view and direct chat entry points.

## List Conversations

Use `GET /api/v1/conversations` with `Authorization: Bearer <accessToken>`.

Query parameters:

- `limit`: optional, defaults to 50, maximum 100.
- `before`: optional RFC3339 timestamp. When present, only conversations updated before this timestamp are returned.

Each item includes:

- `members`: the conversation participants.
- `createdBy`: current owner user ID, used by group management controls.
- `lastMessage`: the most recent message, or `null` if the conversation is empty.
- `unreadCount`: messages from other members that the current user has not marked read.

The list is ordered by latest activity first.
For direct conversations with an empty stored title, the summary title is derived from the other member's display name or username.

## Direct Conversation

Use `POST /api/v1/conversations/direct` with `targetUserId` to fetch or create a one-to-one conversation with an accepted friend.

## Group Conversation

Use `POST /api/v1/conversations/group` to create a group chat. The creator is added automatically, and every `memberIds` entry must be an accepted friend.

```json
{
  "title": "Weekend Plans",
  "memberIds": ["<bobUserId>", "<carolUserId>"]
}
```

The response uses the same `Conversation` shape as direct conversations, with `type` set to `group`. Group messages, unread counts, sync, and WebSocket delivery reuse the normal conversation membership rules.

## Group Management

The group creator is the current owner. Owner transfer and administrator roles are not implemented yet.

- Rename: `PATCH /api/v1/conversations/{conversationId}` with `{ "title": "New Name" }`.
- Transfer owner: `PATCH /api/v1/conversations/{conversationId}/owner` with `{ "targetUserId": "<userId>" }`.
- Add members: `POST /api/v1/conversations/{conversationId}/members` with `{ "memberIds": ["<userId>"] }`.
- Remove member: `DELETE /api/v1/conversations/{conversationId}/members/{userId}`.
- Leave group: `POST /api/v1/conversations/{conversationId}/leave`.

Only the owner can rename, transfer ownership, add members, or remove members. Added members must be accepted friends of the owner. Non-owner members can leave. The current owner must transfer ownership before leaving.
After transfer, the previous owner becomes a normal member. Each successful management action publishes a `conversation.updated` WebSocket event to relevant members.

## Message History

Use `GET /api/v1/conversations/{conversationId}/messages` to load chat history, and `POST /api/v1/conversations/{conversationId}/read` to advance the read cursor.
