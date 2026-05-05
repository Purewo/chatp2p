CREATE TABLE IF NOT EXISTS user_blocks (
	blocker_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	blocked_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (blocker_id, blocked_id),
	CHECK (blocker_id <> blocked_id)
);

CREATE INDEX IF NOT EXISTS idx_user_blocks_blocked_id ON user_blocks(blocked_id);
