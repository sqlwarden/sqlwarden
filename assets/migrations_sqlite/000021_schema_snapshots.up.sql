ALTER TABLE organizations
    ADD COLUMN schema_snapshots_enabled INTEGER NOT NULL DEFAULT 1;

ALTER TABLE connections
    ADD COLUMN schema_snapshot_policy TEXT NOT NULL DEFAULT 'inherit'
        CHECK (schema_snapshot_policy IN ('inherit', 'disabled'));

CREATE TABLE schema_snapshots (
    id            TEXT     NOT NULL PRIMARY KEY,
    connection_id INTEGER  NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    org_id        INTEGER  REFERENCES organizations(id) ON DELETE CASCADE,
    dialect       TEXT     NOT NULL,
    database_name TEXT     NOT NULL DEFAULT '',
    status        TEXT     NOT NULL CHECK (status IN ('building', 'ready')),
    is_active     INTEGER  NOT NULL DEFAULT 0,
    catalog_data  BLOB     NOT NULL,
    generated_at  DATETIME NOT NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at  DATETIME
);

CREATE UNIQUE INDEX idx_schema_snapshots_active_connection
    ON schema_snapshots(connection_id)
    WHERE is_active = 1;
CREATE INDEX idx_schema_snapshots_connection_created
    ON schema_snapshots(connection_id, created_at DESC);
CREATE INDEX idx_schema_snapshots_org
    ON schema_snapshots(org_id);

CREATE TABLE schema_snapshot_objects (
    snapshot_id TEXT NOT NULL REFERENCES schema_snapshots(id) ON DELETE CASCADE,
    namespace   TEXT NOT NULL,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    object_data BLOB NOT NULL,
    PRIMARY KEY (snapshot_id, namespace, kind, name)
);

CREATE TABLE schema_snapshot_relationships (
    snapshot_id       TEXT NOT NULL REFERENCES schema_snapshots(id) ON DELETE CASCADE,
    namespace         TEXT NOT NULL,
    relationship_data BLOB NOT NULL,
    PRIMARY KEY (snapshot_id, namespace)
);
