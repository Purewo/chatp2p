PRAGMA defer_foreign_keys = ON;

CREATE TABLE conversations_new (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL CHECK (type IN ('direct', 'group')),
	title TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

INSERT INTO conversations_new (id, type, title, created_by, created_at, updated_at)
SELECT id, type, title, created_by, created_at, updated_at
FROM conversations;

DROP TABLE conversations;
ALTER TABLE conversations_new RENAME TO conversations;

PRAGMA foreign_key_check;
