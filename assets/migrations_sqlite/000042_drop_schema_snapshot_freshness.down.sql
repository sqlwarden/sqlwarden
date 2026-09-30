ALTER TABLE instance_settings ADD COLUMN schema_snapshot_freshness_seconds INTEGER NOT NULL DEFAULT 86400 CHECK (schema_snapshot_freshness_seconds > 0 AND schema_snapshot_freshness_seconds <= 9223372036);
ALTER TABLE organization_runtime_settings ADD COLUMN schema_snapshot_freshness_seconds INTEGER CHECK (schema_snapshot_freshness_seconds > 0 AND schema_snapshot_freshness_seconds <= 9223372036);
