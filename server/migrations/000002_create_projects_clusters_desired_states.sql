CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX projects_user_id_idx ON projects (user_id) WHERE deleted_at IS NULL;

CREATE TABLE clusters (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id),
    host_id TEXT NOT NULL REFERENCES hosts (id),
    name TEXT NOT NULL,
    postgres_version TEXT NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    replica_count INTEGER NOT NULL DEFAULT 0 CHECK (replica_count >= 0),
    pgbouncer_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE INDEX clusters_project_id_idx ON clusters (project_id) WHERE deleted_at IS NULL;
CREATE INDEX clusters_host_id_idx ON clusters (host_id) WHERE deleted_at IS NULL;

CREATE TABLE desired_states (
    id BIGSERIAL PRIMARY KEY,
    host_id TEXT NOT NULL REFERENCES hosts (id),
    cluster_id TEXT NOT NULL REFERENCES clusters (id),
    operation TEXT NOT NULL CHECK (operation IN ('upsert', 'delete')),
    state JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX desired_states_cluster_version_idx
    ON desired_states (cluster_id, id DESC);
CREATE INDEX desired_states_host_version_idx
    ON desired_states (host_id, id DESC);
