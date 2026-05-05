# Messages

Message APIs require `Authorization: Bearer <accessToken>` and a conversation membership.

## Sending Messages

Use `POST /api/v1/conversations/{conversationId}/messages` with:

```json
{
  "type": "text",
  "body": "hello",
  "quoteMessageId": "<optionalMessageId>"
}
```

For `text` messages, empty messages are rejected, and `body` is limited to 4000 characters.
When `quoteMessageId` is present, the quoted message must be in the same conversation, visible to the sender, and not recalled. Responses include `quotedMessage` with the quoted message preview.

Sticker messages use the same endpoint with `type` set to `sticker` and `body` set to a sticker id from the sticker catalog:

```json
{
  "type": "sticker",
  "body": "classic-smile"
}
```

Unknown sticker ids are rejected. Sticker assets and pack metadata are documented in `docs/stickers.md`.

## Message Edit

Use `PATCH /api/v1/conversations/{conversationId}/messages/{messageId}` with:

```json
{
  "body": "updated text"
}
```

Only the original sender can edit their own non-recalled text message. Sticker messages cannot be edited. Empty edits are rejected, and `body` is limited to 4000 characters. The response is the updated `Message`. After edit, `editedAt` is set and `editedBy` contains the user who performed the edit.

## Message Recall

Use `POST /api/v1/conversations/{conversationId}/messages/{messageId}/recall` to recall a message. Only the original sender can recall their own message.

The response is the updated `Message`. After recall, `body` is an empty string, `recalledAt` is set, and `recalledBy` contains the user who performed the recall. Frontend clients should render a recalled-message placeholder whenever `recalledAt` is present.

## Forward, Favorite, and Delete For Me

Single-message actions:

- Forward: `POST /api/v1/conversations/{conversationId}/messages/{messageId}/forward` with `{ "targetConversationId": "<conversationId>" }`.
- Favorite: `POST /api/v1/conversations/{conversationId}/messages/{messageId}/favorite`.
- Unfavorite: `DELETE /api/v1/conversations/{conversationId}/messages/{messageId}/favorite`.
- Delete for me: `DELETE /api/v1/conversations/{conversationId}/messages/{messageId}`.

Multi-select actions use a maximum of 50 message ids:

- Forward: `POST /api/v1/conversations/{conversationId}/messages/forward` with `{ "messageIds": ["<messageId>"], "targetConversationId": "<conversationId>" }`.
- Favorite: `POST /api/v1/conversations/{conversationId}/messages/favorite` with `{ "messageIds": ["<messageId>"] }`.
- Delete for me: `POST /api/v1/conversations/{conversationId}/messages/delete` with `{ "messageIds": ["<messageId>"] }`.

Delete for me only hides messages from the current user's history, conversation last-message preview, unread counts, and favorites. It does not remove the message for other members. Use recall when the sender needs the message to disappear for everyone.

Use `GET /api/v1/message-favorites?limit=50` to list favorited messages. Recalled messages cannot be newly favorited or forwarded.

## History

Use `GET /api/v1/conversations/{conversationId}/messages` to load history. The response is ordered oldest to newest within the returned page.

Query parameters:

- `limit`: optional, defaults to 50, maximum 100.
- `cursor`: optional opaque pagination cursor from the previous response's `nextCursor`.
- `before`: optional legacy RFC3339 timestamp. Prefer `cursor` for stable pagination.

The response includes `nextCursor` when older messages are still available. Do not combine `cursor` and `before`.

Cursor pagination uses the same composite order as the history query, including `createdAt` and `id`, so messages with identical timestamps still page consistently.

## Read Receipts

Use `POST /api/v1/conversations/{conversationId}/read` with:

```json
{
  "messageId": "<messageId>"
}
```

The service marks all received messages in that conversation through the target message as read by the current user. The sender can see read receipts in each message's `readBy` array.

## Realtime Follow-Up

REST is the source of truth for persistence and recovery. WebSocket delivery should publish the same `Message` shape used by OpenAPI so frontend code does not maintain separate DTOs.

Message create, edit, and recall publish `message.created`, `message.edited`, and `message.recalled` with the updated `Message` payload. Reconnect sync also returns edited and recalled messages because those actions update the message's `updatedAt`.
