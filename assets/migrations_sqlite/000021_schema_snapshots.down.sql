DROP TABLE IF EXISTS schema_snapshot_relationships;
DROP TABLE IF EXISTS schema_snapshot_objects;
DROP TABLE IF EXISTS schema_snapshots;

ALTER TABLE connections DROP COLUMN schema_snapshot_policy;
ALTER TABLE organizations DROP COLUMN schema_snapshots_enabled;
