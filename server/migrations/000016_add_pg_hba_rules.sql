ALTER TABLE clusters
    ADD COLUMN pg_hba_rules JSONB NOT NULL DEFAULT '[
      {"type":"host","database":"all","user":"all","address":"0.0.0.0/0","method":"reject"},
      {"type":"host","database":"all","user":"all","address":"::/0","method":"reject"}
    ]'::jsonb
    CHECK (jsonb_typeof(pg_hba_rules) = 'array');

-- Existing clusters were initialized with host trust. Preserve that shipped
-- behavior until their owners explicitly replace it; new clusters use the
-- deny-by-default column default above.
UPDATE clusters
SET pg_hba_rules = '[
  {"type":"host","database":"all","user":"all","address":"0.0.0.0/0","method":"trust"},
  {"type":"host","database":"all","user":"all","address":"::/0","method":"trust"}
]'::jsonb;

INSERT INTO desired_states (host_id, cluster_id, operation, state)
SELECT c.host_id, c.id, 'upsert', jsonb_set(
    latest.state,
    '{pg_hba}',
    jsonb_build_object('rules', c.pg_hba_rules),
    true
)
FROM clusters c
JOIN LATERAL (
    SELECT state
    FROM desired_states
    WHERE cluster_id = c.id AND operation = 'upsert'
    ORDER BY id DESC
    LIMIT 1
) latest ON true
WHERE c.deleted_at IS NULL
  AND NOT (latest.state ? 'pg_hba');
