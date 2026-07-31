CREATE TYPE restore_operation_mode AS ENUM ('in_place', 'clone');
CREATE TYPE restore_operation_intent AS ENUM ('preflight', 'execute', 'cancel', 'rollback', 'finalize');
CREATE TYPE restore_operation_status AS ENUM (
    'pending', 'ready', 'running', 'succeeded', 'failed',
    'cancelled', 'rolled_back', 'finalized'
);

CREATE TABLE restore_operations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id),
    host_id TEXT NOT NULL REFERENCES hosts (id),
    source_cluster_id TEXT NOT NULL REFERENCES clusters (id),
    target_cluster_id TEXT,
    target_cluster_name TEXT,
    target_spec JSONB NOT NULL DEFAULT 'null'::jsonb,
    mode restore_operation_mode NOT NULL,
    intent restore_operation_intent NOT NULL DEFAULT 'preflight',
    status restore_operation_status NOT NULL DEFAULT 'pending',
    target_time TIMESTAMPTZ NOT NULL,
    request_fingerprint TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    requested_by_user_id TEXT NOT NULL REFERENCES users (id),
    report_sequence BIGINT NOT NULL DEFAULT 0 CHECK (report_sequence >= 0),
    report JSONB NOT NULL DEFAULT 'null'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finalized_at TIMESTAMPTZ,
    CHECK (
        (mode = 'in_place' AND target_cluster_id IS NULL AND target_cluster_name IS NULL AND target_spec = 'null'::jsonb)
        OR
        (mode = 'clone' AND target_cluster_id IS NOT NULL AND target_cluster_name <> '' AND target_spec <> 'null'::jsonb)
    ),
    UNIQUE (requested_by_user_id, idempotency_key),
    UNIQUE (target_cluster_id)
);

CREATE UNIQUE INDEX restore_operations_active_source_idx
    ON restore_operations (source_cluster_id)
    WHERE status NOT IN ('cancelled', 'finalized')
      AND NOT (mode = 'clone' AND status = 'succeeded');

CREATE UNIQUE INDEX restore_operations_active_target_idx
    ON restore_operations (target_cluster_id)
    WHERE target_cluster_id IS NOT NULL
      AND status NOT IN ('cancelled', 'finalized')
      AND NOT (mode = 'clone' AND status = 'succeeded');

CREATE INDEX restore_operations_project_idx
    ON restore_operations (project_id, created_at DESC, id);
CREATE INDEX restore_operations_host_actionable_idx
    ON restore_operations (host_id, updated_at, id)
    WHERE status NOT IN ('cancelled', 'finalized')
      AND NOT (mode = 'clone' AND status = 'succeeded');

CREATE TABLE restore_operation_events (
    id BIGSERIAL PRIMARY KEY,
    operation_id TEXT NOT NULL REFERENCES restore_operations (id),
    host_id TEXT NOT NULL REFERENCES hosts (id),
    event_type TEXT NOT NULL CHECK (event_type IN ('created', 'intent_changed', 'agent_report', 'finalized')),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX restore_operation_events_operation_idx
    ON restore_operation_events (operation_id, id);
CREATE INDEX restore_operation_events_host_revision_idx
    ON restore_operation_events (host_id, id DESC);
