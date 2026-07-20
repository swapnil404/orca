CREATE TABLE hosts (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token_hash BYTEA NOT NULL UNIQUE,
    token_expires_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'never_connected'
        CHECK (status IN ('never_connected', 'online', 'offline')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    connected_at TIMESTAMPTZ
);

CREATE INDEX hosts_user_id_idx ON hosts (user_id);
