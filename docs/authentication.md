# Authentication

Authentication uses REST endpoints under `/api/v1/auth` and Bearer tokens for protected APIs.

## Endpoints

- `POST /api/v1/auth/register` creates a user and returns an access token.
- `POST /api/v1/auth/login` verifies username/password credentials and returns an access token.
- `GET /api/v1/users/me` returns the authenticated user's profile and requires `Authorization: Bearer <token>`.
- `PATCH /api/v1/users/me` updates the authenticated user's `displayName`, `avatarUrl`, or `bio`.

## Account Rules

- Usernames are normalized to lowercase and must be 3-32 characters.
- Usernames may contain letters, digits, `_`, `-`, and `.`.
- Passwords must be 8-72 bytes because the current password hasher is bcrypt.
- `displayName` is optional; if omitted, it defaults to the normalized username.
- `avatarUrl` is optional. Absolute `http(s)` URLs or app-relative paths such as `/avatars/alice.png` are accepted.
- `bio` is optional and limited to 160 characters.

## Token Rules

Tokens are signed as HS256 JWT-style tokens. The payload includes `sub`, `username`, `displayName`, `iat`, `exp`, and `iss`.

Set `JWT_SECRET` outside development. The default secret is only for local runs. `JWT_TTL` defaults to `24h`.

## Error Shape

API errors use:

```json
{
  "code": "invalid_request",
  "message": "request validation failed"
}
```
