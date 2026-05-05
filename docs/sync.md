# Offline Sync

Use sync after app resume or WebSocket reconnect to recover missed data.

## Endpoint

```http
GET /api/v1/sync?since=<RFC3339>&limit=50
Authorization: Bearer <accessToken>
```

`since` is required. Clients should store the previous response's `serverTime` and send it on the next sync.

## Response

```json
{
  "serverTime": "2026-05-04T03:20:00Z",
  "conversations": [],
  "messages": []
}
```

- `conversations`: current conversation summaries, including archived conversations, `lastMessage`, `unreadCount`, and per-user settings such as `pinnedAt`, `mutedUntil`, and `archivedAt`.
- `messages`: messages created or updated after `since`, ordered oldest to newest by update time.
- `serverTime`: next sync cursor after the response is processed.

## Client Flow

1. On first app load, fetch `GET /api/v1/conversations` and per-conversation history as needed.
2. Open `GET /api/v1/ws` for realtime events.
3. After reconnect, call `GET /api/v1/sync` with the last stored `serverTime`.
4. Merge returned messages by `id` to avoid duplicates and refresh existing rows. This is required for edited messages, where `body` changes and `editedAt` is set, and recalled messages, where `body` becomes empty and `recalledAt` is set.
