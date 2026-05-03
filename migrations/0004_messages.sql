CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	sender_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	type TEXT NOT NULL CHECK (type IN ('text')),
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON messages(conversation_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS read_receipts (
	message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	read_at INTEGER NOT NULL,
	PRIMARY KEY (message_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_read_receipts_user ON read_receipts(user_id, read_at DESC);
