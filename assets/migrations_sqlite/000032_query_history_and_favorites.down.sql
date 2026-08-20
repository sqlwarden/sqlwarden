DROP TABLE IF EXISTS query_favorites;
DROP TABLE IF EXISTS query_history;
ALTER TABLE organization_runtime_settings DROP COLUMN query_history_mode;
ALTER TABLE organization_runtime_settings DROP COLUMN query_history_retention_count;
ALTER TABLE organization_runtime_settings DROP COLUMN query_favorites_mode;
ALTER TABLE instance_settings DROP COLUMN query_history_mode;
ALTER TABLE instance_settings DROP COLUMN query_history_retention_count;
ALTER TABLE instance_settings DROP COLUMN query_history_retention_count_max;
ALTER TABLE instance_settings DROP COLUMN query_favorites_mode;
