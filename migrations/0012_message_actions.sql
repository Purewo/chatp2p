ALTER TABLE messages ADD COLUMN quoted_message_id TEXT REFERENCES messages(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS message_user_states (
	message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	favorited_at INTEGER,
	deleted_at INTEGER,
	PRIMARY KEY (message_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_message_user_states_user_favorited
	ON message_user_states(user_id, favorited_at DESC)
	WHERE favorited_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_message_user_states_user_deleted
	ON message_user_states(user_id, deleted_at DESC)
	WHERE deleted_at IS NOT NULL;
