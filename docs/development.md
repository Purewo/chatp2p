# Development Guide

## Local Commands

```sh
go run ./cmd/server
go test ./...
gofmt -w cmd internal
```

The server listens on `:8080` by default. Override settings with environment variables:

```sh
HTTP_ADDR=:8081 APP_ENV=development LOG_LEVEL=debug go run ./cmd/server
```

## Environment Variables

| Name | Default | Purpose |
| --- | --- | --- |
| `APP_NAME` | `chatp2p` | Service name returned by health checks and logs. |
| `APP_ENV` | `development` | Runtime environment label. |
| `HTTP_ADDR` | `:8080` | HTTP bind address. |
| `DB_DRIVER` | `sqlite` | SQL driver name. Local development currently uses SQLite. |
| `DB_DSN` | `file:chatp2p.db` | Database connection string. |
| `JWT_SECRET` | `chatp2p-dev-secret-change-me` | Token signing secret. Override outside local development. |
| `JWT_ISSUER` | `chatp2p` | Token issuer claim. |
| `JWT_TTL` | `24h` | Access token lifetime. |
| `LOG_LEVEL` | `info` | One of `debug`, `info`, `warn`, or `error`. |
| `SHUTDOWN_TIMEOUT` | `5s` | Graceful shutdown timeout. |

## API Contract

Keep `api/openapi.yaml` synchronized with every frontend-facing REST change. Do not merge API behavior that is not documented for frontend integration.

## Local Authentication Flow

```sh
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"alice\",\"password\":\"password123\",\"displayName\":\"Alice\",\"avatarUrl\":\"/avatars/alice.png\",\"bio\":\"Ready to chat\"}"
```

Use the returned `accessToken` to update the profile:

```sh
curl -X PATCH http://localhost:8080/api/v1/users/me \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <accessToken>" \
  -d "{\"displayName\":\"Alice Updated\",\"avatarUrl\":\"\",\"bio\":\"Usually online at night\"}"
```

## Local Friend Flow

Register two users, then use one user's token to search and send a friend request:

```sh
curl "http://localhost:8080/api/v1/users?query=bob" \
  -H "Authorization: Bearer <aliceToken>"

curl -X POST http://localhost:8080/api/v1/friend-requests \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <aliceToken>" \
  -d "{\"targetUserId\":\"<bobUserId>\",\"message\":\"hi\"}"
```

Bob can accept the request:

```sh
curl -X POST http://localhost:8080/api/v1/friend-requests/<requestId>/accept \
  -H "Authorization: Bearer <bobToken>"
```

Either user can remove the friendship later:

```sh
curl -X DELETE http://localhost:8080/api/v1/friends/<bobUserId> \
  -H "Authorization: Bearer <aliceToken>"
```

Users can also block and unblock accounts:

```sh
curl -X POST http://localhost:8080/api/v1/blocks/<bobUserId> \
  -H "Authorization: Bearer <aliceToken>"

curl http://localhost:8080/api/v1/blocks \
  -H "Authorization: Bearer <aliceToken>"

curl -X DELETE http://localhost:8080/api/v1/blocks/<bobUserId> \
  -H "Authorization: Bearer <aliceToken>"
```

## Local Message Flow

Use the `conversation.id` returned from accepting a friend request:

```sh
curl -X POST http://localhost:8080/api/v1/conversations/<conversationId>/messages \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <aliceToken>" \
  -d "{\"type\":\"text\",\"body\":\"hello\"}"

curl http://localhost:8080/api/v1/conversations/<conversationId>/messages \
  -H "Authorization: Bearer <bobToken>"

curl -X PATCH http://localhost:8080/api/v1/conversations/<conversationId>/messages/<messageId> \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <aliceToken>" \
  -d "{\"body\":\"hello, edited\"}"

curl -X POST http://localhost:8080/api/v1/conversations/<conversationId>/read \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <bobToken>" \
  -d "{\"messageId\":\"<messageId>\"}"

curl -X PATCH http://localhost:8080/api/v1/conversations/<conversationId>/settings \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <bobToken>" \
  -d "{\"pinned\":true,\"mutedUntil\":\"2030-01-01T12:00:00Z\",\"archived\":true}"

curl "http://localhost:8080/api/v1/conversations?includeArchived=true" \
  -H "Authorization: Bearer <bobToken>"
```

## WebSocket Check

Connect with the same token used for REST:

```text
ws://localhost:8080/api/v1/ws?accessToken=<accessToken>
```

Each pushed event uses the same payload shapes documented in `docs/websocket.md` and `api/openapi.yaml`.

Clients can publish typing state on the same socket:

```json
{
  "type": "typing.started",
  "data": {
    "conversationId": "<conversationId>"
  }
}
```
