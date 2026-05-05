# Offline Sync

Use sync after app resume or WebSocket reconnect to recover missed data.

## Endpoint

```http
GET /api/v1/sync?cursor=<opaqueCursor>&limit=50
Authorization: Bearer <accessToken>
```

Query parameters:

- `cursor`: preferred opaque sync cursor from the previous response's `nextCursor`.
- `since`: optional legacy RFC3339 timestamp. Prefer `cursor` for stable resume. Do not combine with `cursor`.
- `limit`: optional page size for changed messages, defaults to 50, maximum 100.

## Response

```json
{
  "serverTime": "2026-05-04T03:20:00Z",
  "nextCursor": "<opaqueCursor>",
  "hasMore": false,
  "conversations": [],
  "messages": []
}
```

- `conversations`: current conversation summaries, including archived conversations, `lastMessage`, `unreadCount`, and per-user settings such as `pinnedAt`, `mutedUntil`, and `archivedAt`.
- `messages`: current page of messages created or updated after the supplied sync cursor, ordered oldest to newest by change order.
- `nextCursor`: resume cursor to store after processing the response. When `hasMore` is `true`, call sync again immediately with this cursor.
- `hasMore`: true when more changed messages remain in the current sync window.
- `serverTime`: server snapshot time for the response. Legacy clients may still reuse it as the next `since` value.

## Client Flow

1. On first app load, fetch `GET /api/v1/conversations` and per-conversation history as needed.
2. Open `GET /api/v1/ws` for realtime events.
3. After reconnect, call `GET /api/v1/sync` with the last stored `nextCursor`. Legacy clients may bootstrap with `since`.
4. Merge returned messages by `id` to avoid duplicates and refresh existing rows. This is required for edited messages, where `body` changes and `editedAt` is set, and recalled messages, where `body` becomes empty and `recalledAt` is set.
5. If `hasMore` is `true`, call sync again immediately with the returned `nextCursor` until `hasMore` becomes `false`.
