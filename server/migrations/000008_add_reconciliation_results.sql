ALTER TABLE agent_reports
ADD COLUMN reconciliation_results JSONB NOT NULL DEFAULT '[]'::jsonb;
