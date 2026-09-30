ALTER TABLE connections ADD COLUMN show_all_databases INTEGER NOT NULL DEFAULT 0;

CREATE TABLE schema_nodes (
    connection_id INTEGER  NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    parent_path   TEXT     NOT NULL,
    folder        TEXT     NOT NULL,
    children_data BLOB     NOT NULL,
    fetched_at    DATETIME NOT NULL,
    PRIMARY KEY (connection_id, parent_path, folder)
);

CREATE TABLE schema_objects (
    connection_id INTEGER  NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    scope         TEXT     NOT NULL,
    kind          TEXT     NOT NULL,
    name          TEXT     NOT NULL,
    object_data   BLOB     NOT NULL,
    fetched_at    DATETIME NOT NULL,
    PRIMARY KEY (connection_id, scope, kind, name)
);

CREATE TABLE schema_relationships (
    connection_id INTEGER  NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    scope         TEXT     NOT NULL,
    data          BLOB     NOT NULL,
    fetched_at    DATETIME NOT NULL,
    PRIMARY KEY (connection_id, scope)
);
