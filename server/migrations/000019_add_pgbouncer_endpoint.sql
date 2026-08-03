ALTER TABLE clusters
    ADD COLUMN pgbouncer_publish_address TEXT NOT NULL DEFAULT '127.0.0.1'
        CHECK (pgbouncer_publish_address <> ''),
    ADD COLUMN pgbouncer_publish_port INTEGER NOT NULL DEFAULT 6432
        CHECK (pgbouncer_publish_port BETWEEN 1 AND 65535);

INSERT INTO desired_states (host_id, cluster_id, operation, state)
SELECT c.host_id, c.id, 'upsert', jsonb_set(
    jsonb_set(latest.state, '{pg_bouncer,publish_address}', to_jsonb(c.pgbouncer_publish_address), true),
    '{pg_bouncer,publish_port}', to_jsonb(c.pgbouncer_publish_port), true
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
  AND c.pgbouncer_enabled
  AND latest.state ? 'pg_bouncer';
