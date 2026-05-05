# Messages

Message APIs require `Authorization: Bearer <accessToken>` and a conversation membership.

## Sending Messages

Use `POST /api/v1/conversations/{conversationId}/messages` with:

```json
{
  "type": "text",
  "body": "hello"
}
```

Only `text` messages are supported in the current version. Empty messages are rejected, and `body` is limited to 4000 characters.

## Message Edit

Use `PATCH /api/v1/conversations/{conversationId}/messages/{messageId}` with:

```json
{
  "body": "updated text"
}
```

Only the original sender can edit their own non-recalled message. Empty edits are rejected, and `body` is limited to 4000 characters. The response is the updated `Message`. After edit, `editedAt` is set and `editedBy` contains the user who performed the edit.

## Message Recall

Use `POST /api/v1/conversations/{conversationId}/messages/{messageId}/recall` to recall a message. Only the original sender can recall their own message.

The response is the updated `Message`. After recall, `body` is an empty string, `recalledAt` is set, and `recalledBy` contains the user who performed the recall. Frontend clients should render a recalled-message placeholder whenever `recalledAt` is present.

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
