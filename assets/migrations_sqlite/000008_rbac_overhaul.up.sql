-- Drop old tables (FK order matters; SQLite requires foreign_keys=ON).
DROP TABLE IF EXISTS casbin_rules;
DROP TABLE IF EXISTS access_grants;
DROP TABLE IF EXISTS workspace_roles;
DROP TABLE IF EXISTS connections;
DROP TABLE IF EXISTS workspaces;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TABLE IF EXISTS refresh_tokens;
DROP TABLE IF EXISTS tenant_members;
DROP TABLE IF EXISTS tenant_idp_configs;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS accounts;

CREATE TABLE accounts (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    email      TEXT     NOT NULL UNIQUE,
    name       TEXT     NOT NULL,
    password   TEXT,
    is_active  INTEGER  NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE organizations (
    id         INTEGER  PRIMARY KEY AUTOINCREMENT,
    slug       TEXT     NOT NULL UNIQUE,
    name       TEXT     NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE org_idp_configs (
    id           TEXT    NOT NULL PRIMARY KEY,
    org_id       INTEGER NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    provider     TEXT    NOT NULL,
    display_name TEXT    NOT NULL,
    config       TEXT    NOT NULL,
    sso_required INTEGER NOT NULL DEFAULT 0,
    is_active    INTEGER NOT NULL DEFAULT 1,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE org_members (
    org_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    joined_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (org_id, account_id)
);
CREATE INDEX idx_org_members_account ON org_members(account_id);

CREATE TABLE refresh_tokens (
    id         TEXT    NOT NULL PRIMARY KEY,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT    NOT NULL UNIQUE,
    family     TEXT    NOT NULL,
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    user_agent TEXT,
    ip_address TEXT
);
CREATE INDEX idx_refresh_tokens_account ON refresh_tokens(account_id);
CREATE INDEX idx_refresh_tokens_family  ON refresh_tokens(family);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);

CREATE TABLE instance_settings (
    id                      INTEGER  PRIMARY KEY CHECK (id = 1),
    personal_spaces_enabled BOOLEAN  NOT NULL DEFAULT TRUE,
    created_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE teams (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    slug       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(org_id, slug)
);
CREATE INDEX idx_teams_org ON teams(org_id);

CREATE TABLE team_members (
    team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, account_id)
);
CREATE INDEX idx_team_members_account ON team_members(account_id);

CREATE TABLE workspaces (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id      INTEGER REFERENCES organizations(id) ON DELETE CASCADE,
    owner_type  TEXT    NOT NULL,
    owner_id    INTEGER NOT NULL,
    name        TEXT    NOT NULL,
    description TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workspaces_owner ON workspaces(owner_type, owner_id);

CREATE TABLE environments (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    description  TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workspace_id, name)
);

CREATE TABLE connections (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id   INTEGER NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    environment_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    name           TEXT    NOT NULL,
    driver         TEXT    NOT NULL,
    dsn_encrypted  TEXT    NOT NULL,
    access_mode    TEXT    NOT NULL DEFAULT 'open',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_connections_workspace ON connections(workspace_id);

CREATE TABLE roles (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id      INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    description TEXT,
    scope_type  TEXT    NOT NULL,
    is_builtin  INTEGER NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(org_id, name)
);
CREATE INDEX idx_roles_org ON roles(org_id);

CREATE TABLE role_permissions (
    role_id    INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT    NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE role_bindings (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id        INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_id       INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    subject_type  TEXT    NOT NULL,
    subject_id    INTEGER NOT NULL,
    resource_type TEXT    NOT NULL,
    resource_id   INTEGER NOT NULL,
    expires_at    DATETIME,
    source_type   TEXT,
    source_id     INTEGER,
    created_by    INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(role_id, subject_type, subject_id, resource_type, resource_id)
);
CREATE INDEX idx_role_bindings_resource ON role_bindings(org_id, resource_type, resource_id);
CREATE INDEX idx_role_bindings_subject  ON role_bindings(subject_type, subject_id, org_id);

CREATE TABLE resource_hierarchy (
    child_type  TEXT    NOT NULL,
    child_id    INTEGER NOT NULL,
    parent_type TEXT    NOT NULL,
    parent_id   INTEGER NOT NULL,
    owner_type  TEXT    NOT NULL,
    owner_id    INTEGER NOT NULL,
    PRIMARY KEY (child_type, child_id, parent_type, parent_id)
);
CREATE INDEX idx_resource_hierarchy_child ON resource_hierarchy(child_type, child_id);
