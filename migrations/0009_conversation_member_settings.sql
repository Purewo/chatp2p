ALTER TABLE conversation_members ADD COLUMN pinned_at INTEGER;
ALTER TABLE conversation_members ADD COLUMN muted_until INTEGER;
ALTER TABLE conversation_members ADD COLUMN archived_at INTEGER;

CREATE INDEX IF NOT EXISTS idx_conversation_members_user_archived ON conversation_members(user_id, archived_at);
CREATE INDEX IF NOT EXISTS idx_conversation_members_user_pinned ON conversation_members(user_id, pinned_at DESC);
