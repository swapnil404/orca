ALTER TABLE clusters
    ADD CONSTRAINT clusters_project_id_id_unique UNIQUE (project_id, id);

CREATE TABLE alert_rules (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    cluster_id TEXT,
    metric_name TEXT NOT NULL CHECK (metric_name <> ''),
    comparison TEXT NOT NULL CHECK (comparison IN ('gt', 'gte', 'lt', 'lte', 'eq', 'neq')),
    threshold DOUBLE PRECISION NOT NULL,
    duration_before_firing_seconds BIGINT NOT NULL DEFAULT 0
        CHECK (duration_before_firing_seconds >= 0),
    current_state TEXT NOT NULL DEFAULT 'ok'
        CHECK (current_state IN ('ok', 'firing')),
    last_transition_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    FOREIGN KEY (project_id, cluster_id)
        REFERENCES clusters (project_id, id) ON DELETE CASCADE
);

CREATE INDEX alert_rules_project_id_idx ON alert_rules (project_id);
CREATE INDEX alert_rules_cluster_id_idx ON alert_rules (cluster_id) WHERE cluster_id IS NOT NULL;
