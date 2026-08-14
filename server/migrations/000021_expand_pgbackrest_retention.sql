ALTER TABLE clusters
    ALTER COLUMN pgbackrest_retention_full TYPE BIGINT,
    ALTER COLUMN pgbackrest_retention_diff TYPE BIGINT;
