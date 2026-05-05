# Next Session Plan

Target date: 2026-05-06.

This is the handoff note for continuing product work after the message action APIs. Image/file messages and voice/video calls are intentionally deferred.

## Current Baseline

- Public backend: `https://chatp2p.ma1.gameuniverse.top`
- Latest pushed commit at handoff: `a23178a feat: add message actions and live docs`
- OpenAPI endpoint: `GET /api/v1/docs/openapi.yaml`
- Recent docs endpoint: `GET /api/v1/docs/recent?limit=5`
- Message actions now available: quote, forward, favorite, multi-select batch actions, edit, recall, read receipts, delete for current user.
- WebSocket already documents and tests `presence.updated`, `typing.started`, and `typing.stopped`.

## First Hour

Start with frontend integration checks before adding new backend behavior:

1. Register or log in two users.
2. Create a direct conversation.
3. Verify message send, quote, edit, recall, forward, favorite, unfavorite, batch favorite, batch delete, and local-only delete.
4. Confirm frontend handles local delete correctly: it must remove the message only from the current user's view and must not render it as recalled.
5. Confirm `GET /api/v1/docs/openapi.yaml` and `GET /api/v1/docs/recent?limit=5` are usable by the frontend tooling.

## Priority 1: Realtime Polish

Presence and typing already exist as WebSocket events. The next useful work is to make them product-ready:

- Frontend should show friend online/offline state from `presence.updated`.
- Frontend should show "typing" state from `typing.started` and clear it on `typing.stopped`, message send, conversation switch, timeout, or socket close.
- Backend gap to evaluate: whether the frontend needs an initial presence snapshot after page load. If yes, add a REST endpoint such as `GET /api/v1/presence/friends` or include presence in the conversations/friends response.
- Backend gap to evaluate: whether to persist `lastActiveAt`. If yes, update user presence state on disconnect and document it.

Acceptance:

- Opening two browser sessions shows online/offline changes without refresh.
- Typing indicator appears only for other conversation members and clears reliably.
- Reconnect recovery does not leave stale typing indicators.

## Priority 2: Message Reactions

This is the best next feature for chat feel. It is lighter than files and calls but makes conversations more alive.

Proposed REST API:

- `POST /api/v1/conversations/{conversationId}/messages/{messageId}/reactions`
- `DELETE /api/v1/conversations/{conversationId}/messages/{messageId}/reactions/{emoji}`

Proposed create body:

```json
{
  "emoji": "like"
}
```

Use stable reaction keys instead of raw arbitrary Unicode first, for example:

- `like`
- `laugh`
- `heart`
- `wow`
- `sad`
- `angry`

Proposed `Message` addition:

```json
{
  "reactions": [
    {
      "emoji": "like",
      "count": 2,
      "users": [
        {
          "id": "<userId>",
          "username": "alice",
          "displayName": "Alice"
        }
      ],
      "reactedByMe": true
    }
  ]
}
```

Storage sketch:

```sql
CREATE TABLE message_reactions (
  message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  emoji TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (message_id, user_id, emoji)
);
```

Realtime events:

- `message.reaction_added`
- `message.reaction_removed`

Rules:

- Only conversation members can react.
- Locally deleted messages should not be reactable by that user.
- Recalled messages should not accept new reactions.
- Reactions should be returned by message history, sync, and favorite list if the frontend needs a consistent `Message` shape.

Acceptance:

- A user can add/remove a reaction.
- Other members receive realtime updates.
- Message history reload shows the correct counts.
- Duplicate reaction from the same user does not double count.

## Priority 3: Message Search

Basic search is more useful than advanced media work at this stage.

Proposed API:

- `GET /api/v1/messages/search?q=<text>&conversationId=<optional>&limit=20`

Rules:

- Search only conversations the current user belongs to.
- Exclude messages locally deleted by the current user.
- Exclude recalled message bodies.
- Start with text messages only; sticker/file/image search can come later.

Acceptance:

- Current conversation search works.
- Global search across joined conversations works.
- Results include enough context for the frontend to jump to the conversation and message.

## Priority 4: QQ-Like Social Polish

These are good follow-up features after reactions/search:

- Friend remark names: let me set my own display alias for a friend.
- Friend groups: custom contact groups such as Friends, Work, Family.
- Group member nicknames: per-group display alias.
- Group announcements.
- Mentions: `@user` and later `@all`.
- System messages for group rename, member join/leave, owner transfer, and recall/delete notices where appropriate.

## Priority 5: Convenience Features

These can wait until the core realtime and reaction work is stable:

- Conversation drafts, preferably local-first on frontend. Add backend sync only if multi-device draft continuity is required.
- Pinned messages inside a conversation.
- Important messages or highlights for group chats.
- Better notification preferences per conversation.

## Tomorrow's Suggested Order

1. Frontend smoke test against the current production backend.
2. Fix any mismatch in today's message action integration.
3. Decide whether presence needs an initial REST snapshot.
4. Implement message reactions end to end: model, migration, store, service, HTTP, WebSocket, OpenAPI, tests.
5. If reactions finish cleanly, start basic text message search.

## Verification Checklist

Run before any push:

```sh
gofmt -w cmd internal
go test ./...
python3 -c "import yaml; yaml.safe_load(open('api/openapi.yaml')); print('openapi yaml ok')"
git diff --check
```

Deploy checklist:

```sh
go build -o /root/chatp2p/bin/chatp2p-server ./cmd/server
systemctl restart chatp2p.service
systemctl is-active chatp2p.service
curl -fsS https://chatp2p.ma1.gameuniverse.top/healthz
```
