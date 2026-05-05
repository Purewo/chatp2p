# Conversations

Conversation APIs are for the chat home view and direct chat entry points.

## List Conversations

Use `GET /api/v1/conversations` with `Authorization: Bearer <accessToken>`.

Query parameters:

- `limit`: optional, defaults to 50, maximum 100.
- `before`: optional RFC3339 timestamp. When present, only conversations updated before this timestamp are returned.
- `includeArchived`: optional boolean, defaults to `false`. Archived conversations are hidden unless this is `true`.

Each item includes:

- `members`: the conversation participants.
- `createdBy`: current owner user ID, used by group management controls.
- `lastMessage`: the most recent message, or `null` if the conversation is empty.
- `unreadCount`: messages from other members that the current user has not marked read.
- `pinnedAt`, `mutedUntil`, and `archivedAt`: per-user settings when present.

The list is ordered by pinned conversations first, then latest activity.
For direct conversations with an empty stored title, the summary title is derived from the other member's display name or username.

## Conversation Settings

Use `PATCH /api/v1/conversations/{conversationId}/settings` to update the current user's settings for a conversation.

```json
{
  "pinned": true,
  "mutedUntil": "2030-01-01T12:00:00Z",
  "archived": true
}
```

All fields are optional, but at least one must be present. `pinned: false` clears the pin, `archived: false` restores the conversation to the default list, and an empty `mutedUntil` string clears mute.

## Direct Conversation

Use `POST /api/v1/conversations/direct` with `targetUserId` to fetch or create a one-to-one conversation with an accepted friend.

Removing or blocking a friend does not delete an existing direct conversation or its messages. The direct conversation endpoint requires the friendship to exist and no active block between the users, while existing conversation membership continues to control history access.

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

Only the owner can rename, transfer ownership, add members, or remove members. Added members must be accepted friends of the owner and must not have an active block relationship with the owner. Non-owner members can leave. The current owner must transfer ownership before leaving.
After transfer, the previous owner becomes a normal member. Each successful management action publishes a `conversation.updated` WebSocket event to relevant members.

## Message History

Use `GET /api/v1/conversations/{conversationId}/messages` to load chat history, and `POST /api/v1/conversations/{conversationId}/read` to advance the read cursor.
