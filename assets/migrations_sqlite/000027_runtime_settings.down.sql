DROP TABLE IF EXISTS organization_runtime_settings;

ALTER TABLE instance_settings DROP COLUMN error_notification_email;
ALTER TABLE instance_settings DROP COLUMN file_revisions_keep_latest;
ALTER TABLE instance_settings DROP COLUMN file_revisions_enabled;
ALTER TABLE instance_settings DROP COLUMN schema_snapshot_freshness_seconds;
ALTER TABLE instance_settings DROP COLUMN exports_background_max_bytes;
ALTER TABLE instance_settings DROP COLUMN exports_sync_max_bytes;
ALTER TABLE instance_settings DROP COLUMN query_max_result_bytes;
ALTER TABLE instance_settings DROP COLUMN query_max_result_rows;
ALTER TABLE instance_settings DROP COLUMN sessions_revocation_enabled;
ALTER TABLE instance_settings DROP COLUMN jwt_access_token_ttl_seconds;
