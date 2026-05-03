# Repository Guidelines

## Project Structure & Module Organization

This is a backend-only private chat service. Keep the Go service organized by responsibility: `cmd/server/` for the HTTP entrypoint, `internal/` for business logic, storage, HTTP handlers, and realtime delivery, `migrations/` for SQLite schema changes, `api/openapi.yaml` for the frontend-facing REST contract, and `docs/` for protocol notes. Place tests beside the code they exercise using Go's `_test.go` pattern.

## Build, Test, and Development Commands

Use `go run ./cmd/server` to start the local API server, usually on `http://127.0.0.1:8080`. Use `go test ./...` to run all unit and integration tests. Run `gofmt -w <files>` before finishing Go edits. If a `Makefile` is added, keep aliases thin, for example `make dev`, `make test`, and `make lint`.

## Coding Style & Naming Conventions

Use short, lowercase package names such as `auth`, `httpapi`, `model`, `service`, `store`, and `storage`. Keep request/response DTOs close to the handler package and shared domain shapes in `internal/model`. Export names only when another package must use them. Prefer clear domain names like `Conversation`, `Message`, `FriendRequest`, and `ReadReceipt`.

## API & Documentation Requirements

Every frontend-facing REST change must update `api/openapi.yaml` in the same change. Every WebSocket event, payload, authentication flow, or message lifecycle change must be recorded in `docs/` with examples where useful. Treat OpenAPI and protocol docs as part of the implementation, not follow-up work.

## Testing Guidelines

Cover authentication, authorization, friend flows, conversation membership, message lifecycle behavior, sync, and WebSocket events. The current datastore is SQLite, so schema changes belong in ordered files such as `migrations/0006_message_recall.sql`. Name tests by behavior, for example `TestMessageRecallUpdatesMessageAndEnforcesSender`.

## Commit & Pull Request Guidelines

No Git history is present in this workspace. If a repository is initialized, use Conventional Commits such as `feat: add message recall` or `fix: reject unauthorized conversation access`. Pull requests should include a concise summary, linked issue when available, test results, and request examples when API behavior changes. PRs that alter API contracts must mention the updated OpenAPI and documentation files.
