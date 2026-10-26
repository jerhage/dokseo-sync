CREATE TABLE accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE
);

CREATE TABLE devices (
	id TEXT PRIMARY KEY,
	account_id INTEGER NOT NULL REFERENCES accounts(id),
	name TEXT NOT NULL,
	token_hash TEXT NOT NULL UNIQUE,
	created_at INTEGER NOT NULL,
	last_seen_at INTEGER,
	revoked_at INTEGER
);

CREATE INDEX devices_by_account ON devices(account_id);

CREATE TABLE changes (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	account_id INTEGER NOT NULL REFERENCES accounts(id),
	id TEXT NOT NULL,
	kind TEXT NOT NULL,
	record_id TEXT NOT NULL,
	data TEXT,
	updated_at INTEGER NOT NULL,
	device_id TEXT NOT NULL REFERENCES devices(id),
	received_at INTEGER NOT NULL,
	UNIQUE (account_id, id)
);
