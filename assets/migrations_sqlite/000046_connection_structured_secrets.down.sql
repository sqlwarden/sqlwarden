PRAGMA foreign_keys = OFF;

DELETE FROM role_permissions
WHERE permission = 'conn:reveal_secret';

ALTER TABLE organizations ADD COLUMN mask_connection_credentials_on_edit INTEGER NOT NULL DEFAULT 0;
ALTER TABLE organizations DROP COLUMN allow_connection_secret_reveal;

DROP TABLE connection_secrets;

CREATE TABLE connections_old (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id           INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    environment_id         INTEGER NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    name                   TEXT    NOT NULL,
    driver                 TEXT    NOT NULL,
    dsn_encrypted          TEXT    NOT NULL,
    tls_config_encrypted   TEXT,
    ssh_config_encrypted   TEXT,
    access_mode            TEXT    NOT NULL DEFAULT 'open',
    schema_snapshot_policy TEXT    NOT NULL DEFAULT 'inherit'
        CHECK (schema_snapshot_policy IN ('inherit', 'disabled')),
    default_scope          TEXT    NOT NULL DEFAULT '',
    show_system_schemas    INTEGER NOT NULL DEFAULT 0,
    show_all_databases     INTEGER NOT NULL DEFAULT 0,
    created_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO connections_old (
    id, workspace_id, environment_id, name, driver,
    dsn_encrypted, tls_config_encrypted, ssh_config_encrypted,
    access_mode, schema_snapshot_policy, default_scope,
    show_system_schemas, show_all_databases, created_at, updated_at
)
SELECT
    id, workspace_id, environment_id, name, driver,
    COALESCE(dsn_encrypted, ''), tls_config_encrypted, ssh_config_encrypted,
    access_mode, schema_snapshot_policy, default_scope,
    show_system_schemas, show_all_databases, created_at, updated_at
FROM connections;

DROP TABLE connections;
ALTER TABLE connections_old RENAME TO connections;
CREATE INDEX idx_connections_workspace ON connections(workspace_id);

PRAGMA foreign_keys = ON;
