ALTER TABLE schema_snapshots
    RENAME COLUMN catalog_data TO directory_data;

ALTER TABLE schema_snapshot_objects
    RENAME COLUMN namespace TO scope;

ALTER TABLE schema_snapshot_relationships
    RENAME COLUMN namespace TO scope;
