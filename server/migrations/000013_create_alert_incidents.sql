ALTER TABLE alert_rules
    ADD COLUMN severity TEXT NOT NULL DEFAULT 'warning'
        CHECK (severity IN ('info', 'warning', 'critical'));

CREATE TABLE alert_incidents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    comparison TEXT NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    severity TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    fired_at TIMESTAMPTZ NOT NULL,
    resolved_at TIMESTAMPTZ,
    CHECK (resolved_at IS NULL OR resolved_at >= fired_at)
);

CREATE INDEX alert_incidents_project_fired_at_idx
    ON alert_incidents (project_id, fired_at DESC);

CREATE UNIQUE INDEX alert_incidents_open_rule_idx
    ON alert_incidents (rule_id)
    WHERE resolved_at IS NULL;

-- Preserve currently firing rules as open incidents without inventing past resolutions.
INSERT INTO alert_incidents (
    project_id, rule_id, metric_name, comparison, threshold, severity, fired_at
)
SELECT project_id, id, metric_name, comparison, threshold, severity, last_transition_at
FROM alert_rules
WHERE current_state = 'firing';
