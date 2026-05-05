# Architecture Notes

ChatP2P is a backend-only private chat system. Frontend clients will integrate through the REST OpenAPI contract and the WebSocket protocol documents maintained in this repository.

## Current Shape

- Go HTTP service entrypoint: `cmd/server`
- REST routing and handlers: `internal/httpapi`
- Runtime configuration: `internal/config`
- Authentication, password hashing, and token issuing: `internal/auth` and `internal/service`
- User persistence adapter: `internal/store`
- REST contract: `api/openapi.yaml`

## Target Architecture

- REST APIs handle authentication, profiles, conversations, message history, and administrative queries.
- Friend requests create accepted friendships and initialize direct conversations for one-to-one chat. Friendships can be removed or blocked without deleting direct conversations or message history.
- Group conversations share the same membership, message history, read receipt, sync, and WebSocket delivery pipeline as direct conversations.
- Group management is owner-controlled in the current version: the owner can rename, transfer ownership, invite friends, and remove members; non-owner members can leave.
- Per-user conversation settings such as pin, mute, and archive are stored on the membership row so each user can organize the same conversation independently.
- Messages are persisted before realtime delivery and include read receipts for conversation members.
- WebSocket connections handle message delivery, read-state notifications, friend presence, and typing state.
- SQLite is the local development store for the current implementation.
- PostgreSQL is the production target for users, conversations, memberships, messages, and delivery state.
- Redis is the production store for online presence, fan-out coordination, short-lived sessions, and rate limits.
- SQLite may be used for fast local or test runs when the behavior matches production storage semantics.
- User profile data currently includes `displayName`, `avatarUrl`, and `bio`.

## Documentation Rule

Any change to REST endpoints must update `api/openapi.yaml`. Any change to WebSocket events, message lifecycle, auth flow, or storage assumptions must update the relevant file in `docs/`.
