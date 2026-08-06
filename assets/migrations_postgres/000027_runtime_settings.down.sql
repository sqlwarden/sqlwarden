DROP TABLE IF EXISTS organization_runtime_settings;

ALTER TABLE instance_settings
    DROP COLUMN error_notification_email,
    DROP COLUMN file_revisions_keep_latest,
    DROP COLUMN file_revisions_enabled,
    DROP COLUMN schema_snapshot_freshness_seconds,
    DROP COLUMN exports_background_max_bytes,
    DROP COLUMN exports_sync_max_bytes,
    DROP COLUMN query_max_result_bytes,
    DROP COLUMN query_max_result_rows,
    DROP COLUMN sessions_revocation_enabled,
    DROP COLUMN jwt_access_token_ttl_seconds;
