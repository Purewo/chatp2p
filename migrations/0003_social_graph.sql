CREATE TABLE IF NOT EXISTS friend_requests (
	id TEXT PRIMARY KEY,
	requester_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	addressee_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	message TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'declined')),
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	UNIQUE (requester_id, addressee_id)
);

CREATE INDEX IF NOT EXISTS idx_friend_requests_addressee_status ON friend_requests(addressee_id, status);
CREATE INDEX IF NOT EXISTS idx_friend_requests_requester_status ON friend_requests(requester_id, status);

CREATE TABLE IF NOT EXISTS friendships (
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	friend_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	PRIMARY KEY (user_id, friend_id)
);

CREATE INDEX IF NOT EXISTS idx_friendships_friend_id ON friendships(friend_id);

CREATE TABLE IF NOT EXISTS conversations (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL CHECK (type IN ('direct')),
	title TEXT NOT NULL DEFAULT '',
	created_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS conversation_members (
	conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
	user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL DEFAULT 'member',
	joined_at INTEGER NOT NULL,
	PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE IF NOT EXISTS direct_conversations (
	user_a_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	user_b_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	conversation_id TEXT NOT NULL UNIQUE REFERENCES conversations(id) ON DELETE CASCADE,
	PRIMARY KEY (user_a_id, user_b_id)
);
