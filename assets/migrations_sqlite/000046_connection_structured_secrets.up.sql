PRAGMA foreign_keys = OFF;

ALTER TABLE connections ADD COLUMN params TEXT NOT NULL DEFAULT '{}';
ALTER TABLE connections ADD COLUMN tls_config TEXT;
ALTER TABLE connections ADD COLUMN ssh_config TEXT;

CREATE TABLE connections_new (
    id                     INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id           INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    environment_id         INTEGER NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    name                   TEXT    NOT NULL,
    driver                 TEXT    NOT NULL,
    params                 TEXT    NOT NULL DEFAULT '{}',
    tls_config             TEXT,
    ssh_config             TEXT,
    dsn_encrypted          TEXT,
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

INSERT INTO connections_new (
    id, workspace_id, environment_id, name, driver,
    params, tls_config, ssh_config,
    dsn_encrypted, tls_config_encrypted, ssh_config_encrypted,
    access_mode, schema_snapshot_policy, default_scope,
    show_system_schemas, show_all_databases, created_at, updated_at
)
SELECT
    id, workspace_id, environment_id, name, driver,
    params, tls_config, ssh_config,
    dsn_encrypted, tls_config_encrypted, ssh_config_encrypted,
    access_mode, schema_snapshot_policy, default_scope,
    show_system_schemas, show_all_databases, created_at, updated_at
FROM connections;

DROP TABLE connections;
ALTER TABLE connections_new RENAME TO connections;
CREATE INDEX idx_connections_workspace ON connections(workspace_id);

CREATE TABLE connection_secrets (
    connection_id   INTEGER  NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    name            TEXT     NOT NULL,
    source          TEXT     NOT NULL,
    value_encrypted TEXT     NOT NULL,
    key_id          TEXT     NOT NULL,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (connection_id, name)
);

ALTER TABLE organizations ADD COLUMN allow_connection_secret_reveal INTEGER NOT NULL DEFAULT 0;
ALTER TABLE organizations DROP COLUMN mask_connection_credentials_on_edit;

INSERT OR IGNORE INTO role_permissions (role_id, permission)
SELECT id, 'conn:reveal_secret'
FROM roles
WHERE workspace_id IS NULL
  AND is_builtin = 1
  AND name IN ('Owner', 'Administrator');

PRAGMA foreign_keys = ON;
