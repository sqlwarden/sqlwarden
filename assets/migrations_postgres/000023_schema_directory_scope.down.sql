ALTER TABLE schema_snapshot_relationships
    RENAME COLUMN scope TO namespace;

ALTER TABLE schema_snapshot_objects
    RENAME COLUMN scope TO namespace;

ALTER TABLE schema_snapshots
    RENAME COLUMN directory_data TO catalog_data;
