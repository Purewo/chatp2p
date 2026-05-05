# Frontend Integration

This document is the practical frontend guide on top of `api/openapi.yaml`.

## Base URLs

- REST base URL: `http://127.0.0.1:8080`
- WebSocket URL: `ws://127.0.0.1:8080/api/v1/ws?accessToken=<accessToken>`

For browser development on a separate dev server, make sure the backend `CORS_ALLOWED_ORIGINS` environment variable includes the frontend origin, for example `http://localhost:5173`.

## Authentication

1. Register with `POST /api/v1/auth/register` or login with `POST /api/v1/auth/login`.
2. Store `accessToken` and send it as `Authorization: Bearer <accessToken>` on every REST request.
3. Browser WebSocket clients should pass the token in the `accessToken` query parameter.

There is currently no refresh-token flow. The frontend should treat `401` as a sign-out boundary and send the user back to login.

## Initial App Load

1. Call `GET /api/v1/users/me`.
2. Call `GET /api/v1/conversations?limit=50`.
3. Load the active thread with `GET /api/v1/conversations/{conversationId}/messages?limit=50`.
4. Seed the sync cursor once with `GET /api/v1/sync?since=<current RFC3339 time>&limit=1` and store the returned `nextCursor`.
5. Open the WebSocket connection.

## Realtime Recovery

Use REST as the source of truth for reconnect recovery.

1. When the socket reconnects, call `GET /api/v1/sync?cursor=<storedNextCursor>&limit=50`.
2. Merge returned `messages` by `id`.
3. Replace or merge returned `conversations` by `id`.
4. Store the response `nextCursor`.
5. If `hasMore` is `true`, call sync again immediately with the new `nextCursor`.

## Pagination

- Conversation list: use `nextCursor` from `GET /api/v1/conversations`.
- Message history: use `nextCursor` from `GET /api/v1/conversations/{conversationId}/messages`.
- Sync: use `nextCursor` from `GET /api/v1/sync`.

Do not combine `cursor` with the legacy `before` or `since` timestamp parameters.

## Recommended Client State

- `session`: current auth session and `accessToken`
- `me`: current user profile
- `conversationsById`: normalized conversation map
- `conversationOrder`: ordered IDs from the list API
- `messagesByConversationId`: normalized thread cache
- `syncCursor`: last processed sync cursor
- `wsStatus`: `connecting`, `open`, or `closed`

## Useful API Flows

- Search users: `GET /api/v1/users?query=<text>`
- Send friend request: `POST /api/v1/friend-requests`
- Accept friend request: `POST /api/v1/friend-requests/{id}/accept`
- Open direct chat: `POST /api/v1/conversations/direct`
- Send message: `POST /api/v1/conversations/{conversationId}/messages`
- Mark read: `POST /api/v1/conversations/{conversationId}/read`
- Update per-thread settings: `PATCH /api/v1/conversations/{conversationId}/settings`

## Error Handling

All API errors use:

```json
{
  "code": "invalid_request",
  "message": "request validation failed"
}
```

Key cases for frontend handling:

- `400`: validation or bad cursor input
- `401`: token missing or invalid
- `403`: member/permission restriction
- `404`: resource missing or not visible
- `409`: duplicate or state conflict such as repeated recall
