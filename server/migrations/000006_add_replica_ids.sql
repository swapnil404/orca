ALTER TABLE clusters
    ADD COLUMN replica_ids JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE clusters c
SET replica_ids = COALESCE(
    (
        SELECT jsonb_agg(replica->>'id' ORDER BY ordinality)
        FROM desired_states ds
        CROSS JOIN LATERAL jsonb_array_elements(ds.state->'replicas') WITH ORDINALITY AS replicas(replica, ordinality)
        WHERE ds.cluster_id = c.id AND ds.operation = 'upsert'
          AND ds.id = (
              SELECT latest.id
              FROM desired_states latest
              WHERE latest.cluster_id = c.id AND latest.operation = 'upsert'
              ORDER BY latest.id DESC
              LIMIT 1
          )
    ),
    '[]'::jsonb
);
