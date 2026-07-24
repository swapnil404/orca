ALTER TABLE clusters
    ADD COLUMN pgbackrest_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN pgbackrest_repo_path TEXT NOT NULL DEFAULT '',
    ADD COLUMN pgbackrest_retention_full INTEGER NOT NULL DEFAULT 0 CHECK (pgbackrest_retention_full >= 0),
    ADD COLUMN pgbackrest_retention_diff INTEGER NOT NULL DEFAULT 0 CHECK (pgbackrest_retention_diff >= 0),
    ADD COLUMN pgbackrest_full_interval_seconds BIGINT NOT NULL DEFAULT 0 CHECK (pgbackrest_full_interval_seconds >= 0),
    ADD COLUMN pgbackrest_diff_interval_seconds BIGINT NOT NULL DEFAULT 0 CHECK (pgbackrest_diff_interval_seconds >= 0),
    ADD COLUMN pgbackrest_incr_interval_seconds BIGINT NOT NULL DEFAULT 0 CHECK (pgbackrest_incr_interval_seconds >= 0);
