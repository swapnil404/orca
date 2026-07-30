ALTER TABLE agent_reports
ADD COLUMN desired_state_revision TEXT NOT NULL DEFAULT '';

ALTER TABLE cluster_reports
ADD COLUMN desired_state_revision TEXT NOT NULL DEFAULT '',
ADD COLUMN reconciliation_results JSONB NOT NULL DEFAULT '[]'::jsonb;
