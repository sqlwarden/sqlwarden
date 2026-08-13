ALTER TABLE organizations
    ADD COLUMN schema_snapshots_enabled BOOLEAN NOT NULL DEFAULT TRUE;

ALTER TABLE connections
    ADD COLUMN schema_snapshot_policy TEXT NOT NULL DEFAULT 'inherit'
        CHECK (schema_snapshot_policy IN ('inherit', 'disabled'));

CREATE TABLE schema_snapshots (
    id            TEXT        PRIMARY KEY,
    connection_id BIGINT      NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    org_id        BIGINT      REFERENCES organizations(id) ON DELETE CASCADE,
    dialect       TEXT        NOT NULL,
    database_name TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL CHECK (status IN ('building', 'ready')),
    is_active     BOOLEAN     NOT NULL DEFAULT FALSE,
    catalog_data  BYTEA       NOT NULL,
    generated_at  TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_schema_snapshots_active_connection
    ON schema_snapshots(connection_id)
    WHERE is_active = TRUE;
CREATE INDEX idx_schema_snapshots_connection_created
    ON schema_snapshots(connection_id, created_at DESC);
CREATE INDEX idx_schema_snapshots_org
    ON schema_snapshots(org_id);

CREATE TABLE schema_snapshot_objects (
    snapshot_id TEXT NOT NULL REFERENCES schema_snapshots(id) ON DELETE CASCADE,
    namespace   TEXT NOT NULL,
    kind        TEXT NOT NULL,
    name        TEXT NOT NULL,
    object_data BYTEA NOT NULL,
    PRIMARY KEY (snapshot_id, namespace, kind, name)
);

CREATE TABLE schema_snapshot_relationships (
    snapshot_id       TEXT NOT NULL REFERENCES schema_snapshots(id) ON DELETE CASCADE,
    namespace         TEXT NOT NULL,
    relationship_data BYTEA NOT NULL,
    PRIMARY KEY (snapshot_id, namespace)
);
