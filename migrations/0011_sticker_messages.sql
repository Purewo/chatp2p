PRAGMA defer_foreign_keys = ON;

CREATE TABLE read_receipts_next (
	message_id TEXT NOT NULL,
	user_id TEXT NOT NULL,
	read_at INTEGER NOT NULL,
	PRIMARY KEY (message_id, user_id)
);

INSERT INTO read_receipts_next (message_id, user_id, read_at)
SELECT message_id, user_id, read_at
FROM read_receipts;

CREATE TABLE message_changes_next (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_id TEXT NOT NULL,
	changed_at INTEGER NOT NULL
);

INSERT INTO message_changes_next (id, message_id, changed_at)
SELECT id, message_id, changed_at
FROM message_changes;

DROP TABLE message_changes;
DROP TABLE read_receipts;

CREATE TABLE messages_next (
	id TEXT PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	sender_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	type TEXT NOT NULL CHECK (type IN ('text', 'sticker')),
	body TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	recalled_at INTEGER,
	recalled_by TEXT REFERENCES users(id) ON DELETE SET NULL,
	edited_at INTEGER,
	edited_by TEXT REFERENCES users(id) ON DELETE SET NULL
);

INSERT INTO messages_next (
	id, conversation_id, sender_id, type, body, created_at, updated_at,
	recalled_at, recalled_by, edited_at, edited_by
)
SELECT
	id, conversation_id, sender_id, type, body, created_at, updated_at,
	recalled_at, recalled_by, edited_at, edited_by
FROM messages;

DROP TABLE messages;
ALTER TABLE messages_next RENAME TO messages;

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON messages(conversation_id, created_at DESC, id DESC);

CREATE TABLE read_receipts (
	message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	read_at INTEGER NOT NULL,
	PRIMARY KEY (message_id, user_id)
);

INSERT INTO read_receipts (message_id, user_id, read_at)
SELECT message_id, user_id, read_at
FROM read_receipts_next;

CREATE INDEX IF NOT EXISTS idx_read_receipts_user ON read_receipts(user_id, read_at DESC);

CREATE TABLE message_changes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	changed_at INTEGER NOT NULL
);

INSERT INTO message_changes (id, message_id, changed_at)
SELECT id, message_id, changed_at
FROM message_changes_next;

CREATE INDEX IF NOT EXISTS idx_message_changes_message_id_id ON message_changes(message_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_message_changes_changed_at_id ON message_changes(changed_at, id DESC);

DROP TABLE message_changes_next;
DROP TABLE read_receipts_next;

PRAGMA foreign_key_check;
