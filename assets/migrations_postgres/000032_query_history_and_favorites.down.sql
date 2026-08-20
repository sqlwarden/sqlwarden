DROP TABLE IF EXISTS query_favorites;
DROP TABLE IF EXISTS query_history;

ALTER TABLE organization_runtime_settings
    DROP COLUMN IF EXISTS query_history_mode,
    DROP COLUMN IF EXISTS query_history_retention_count,
    DROP COLUMN IF EXISTS query_favorites_mode;

ALTER TABLE instance_settings
    DROP COLUMN IF EXISTS query_history_mode,
    DROP COLUMN IF EXISTS query_history_retention_count,
    DROP COLUMN IF EXISTS query_history_retention_count_max,
    DROP COLUMN IF EXISTS query_favorites_mode;
