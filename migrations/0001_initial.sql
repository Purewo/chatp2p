CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	display_name TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users(username);
