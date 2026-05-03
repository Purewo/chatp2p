# WebSocket

Open the event stream at `GET /api/v1/ws`.

## Authentication

Preferred:

```text
Authorization: Bearer <accessToken>
```

Fallback for browser clients:

```text
/api/v1/ws?accessToken=<accessToken>
```

## Event Envelope

All events use:

```json
{
  "type": "message.created",
  "data": {},
  "sentAt": "2026-05-04T02:00:00Z"
}
```

## Event Types

- `message.created`: sent after a message is persisted.
- `message.edited`: sent after the original sender edits a message. `data` is the updated `Message`.
- `message.recalled`: sent after the original sender recalls a message. `data` is the updated `Message`.
- `message.read`: sent after a user marks a message as read.
- `conversation.updated`: sent after a group conversation is renamed, ownership is transferred, members are added or removed, or a member leaves.
- `presence.updated`: sent to friends when a user goes online or offline.
- `typing.started`: sent to other conversation members when a user starts typing.
- `typing.stopped`: sent to other conversation members when a user stops typing.

## Client Events

Clients may send typing events through the same WebSocket connection:

```json
{
  "type": "typing.started",
  "data": {
    "conversationId": "<conversationId>"
  }
}
```

Use `typing.stopped` with the same payload when the input is cleared, submitted, or idle.

## Notes

- The server accepts the connection only after token validation.
- Message create, edit, recall, and read events are broadcast to all members of the conversation, including the actor.
- Edited message payloads keep the original `id`; `body` contains the updated text, and `editedAt` and `editedBy` are present.
- Recalled message payloads keep the original `id`; `body` is empty and `recalledAt` is present.
- Conversation update events are broadcast to the relevant conversation members, and the payload includes the updated conversation plus the actor and affected member list when applicable.
- For `owner_transferred`, the `member` field carries the new owner.
- Typing events are broadcast to other conversation members, not echoed to the sender.
- Presence is emitted only when a user's first connection opens or last connection closes.
- REST remains the source of truth for recovery and pagination.
