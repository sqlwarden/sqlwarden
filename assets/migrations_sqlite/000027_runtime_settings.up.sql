ALTER TABLE instance_settings ADD COLUMN jwt_access_token_ttl_seconds INTEGER NOT NULL DEFAULT 86400 CHECK (jwt_access_token_ttl_seconds > 0 AND jwt_access_token_ttl_seconds <= 9223372036);
ALTER TABLE instance_settings ADD COLUMN sessions_revocation_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE instance_settings ADD COLUMN query_max_result_rows INTEGER NOT NULL DEFAULT 10000 CHECK (query_max_result_rows > 0);
ALTER TABLE instance_settings ADD COLUMN query_max_result_bytes INTEGER NOT NULL DEFAULT 26214400 CHECK (query_max_result_bytes > 0);
ALTER TABLE instance_settings ADD COLUMN exports_sync_max_bytes INTEGER NOT NULL DEFAULT 104857600 CHECK (exports_sync_max_bytes > 0);
ALTER TABLE instance_settings ADD COLUMN exports_background_max_bytes INTEGER NOT NULL DEFAULT 0 CHECK (exports_background_max_bytes >= 0);
ALTER TABLE instance_settings ADD COLUMN schema_snapshot_freshness_seconds INTEGER NOT NULL DEFAULT 86400 CHECK (schema_snapshot_freshness_seconds > 0 AND schema_snapshot_freshness_seconds <= 9223372036);
ALTER TABLE instance_settings ADD COLUMN file_revisions_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE instance_settings ADD COLUMN file_revisions_keep_latest INTEGER NOT NULL DEFAULT 50 CHECK (file_revisions_keep_latest >= 0);
ALTER TABLE instance_settings ADD COLUMN error_notification_email TEXT NOT NULL DEFAULT '';

INSERT INTO instance_settings (id, instance_name)
VALUES (1, 'SQLWarden')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE organization_runtime_settings (
    org_id                                    INTEGER PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    query_max_result_rows                     INTEGER CHECK (query_max_result_rows > 0),
    query_max_result_bytes                    INTEGER CHECK (query_max_result_bytes > 0),
    exports_sync_max_bytes                    INTEGER CHECK (exports_sync_max_bytes > 0),
    exports_background_max_bytes              INTEGER CHECK (exports_background_max_bytes >= 0),
    schema_snapshot_freshness_seconds         INTEGER CHECK (schema_snapshot_freshness_seconds > 0 AND schema_snapshot_freshness_seconds <= 9223372036),
    file_revisions_enabled                    INTEGER,
    file_revisions_keep_latest                INTEGER CHECK (file_revisions_keep_latest >= 0),
    created_at                                TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                                TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
