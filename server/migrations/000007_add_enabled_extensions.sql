ALTER TABLE clusters
    ADD COLUMN enabled_extensions JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE clusters c
SET enabled_extensions = COALESCE(
    (
        SELECT ds.state->'enabled_extensions'
        FROM desired_states ds
        WHERE ds.cluster_id = c.id AND ds.operation = 'upsert'
        ORDER BY ds.id DESC
        LIMIT 1
    ),
    '[]'::jsonb
);
