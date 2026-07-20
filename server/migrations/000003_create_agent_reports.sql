CREATE TABLE agent_reports (
    host_id TEXT PRIMARY KEY REFERENCES hosts (id) ON DELETE CASCADE,
    actual_state JSONB NOT NULL,
    health_report JSONB NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE cluster_reports (
    host_id TEXT NOT NULL REFERENCES hosts (id) ON DELETE CASCADE,
    cluster_id TEXT NOT NULL REFERENCES clusters (id) ON DELETE CASCADE,
    actual_state JSONB NOT NULL,
    health_status TEXT NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (host_id, cluster_id)
);

CREATE INDEX cluster_reports_cluster_id_idx ON cluster_reports (cluster_id);
