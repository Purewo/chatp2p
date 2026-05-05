CREATE TABLE IF NOT EXISTS message_changes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	message_id TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
	changed_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_message_changes_message_id_id ON message_changes(message_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_message_changes_changed_at_id ON message_changes(changed_at, id DESC);

INSERT INTO message_changes (message_id, changed_at)
SELECT m.id, m.updated_at
FROM messages m
WHERE NOT EXISTS (
	SELECT 1
	FROM message_changes mc
	WHERE mc.message_id = m.id
)
ORDER BY m.updated_at ASC, m.id ASC;
