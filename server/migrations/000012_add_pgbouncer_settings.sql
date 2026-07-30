ALTER TABLE clusters
    ADD COLUMN pgbouncer_pool_mode TEXT NOT NULL DEFAULT 'transaction'
        CHECK (pgbouncer_pool_mode IN ('session', 'transaction', 'statement')),
    ADD COLUMN pgbouncer_max_connections INTEGER NOT NULL DEFAULT 100
        CHECK (pgbouncer_max_connections > 0);
