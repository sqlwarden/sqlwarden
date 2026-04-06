-- Drop old tables (FK order matters).
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

-- Core identity tables.
CREATE TABLE accounts (
    id         BIGSERIAL   PRIMARY KEY,
    email      TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    password   TEXT,
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE organizations (
    id         BIGSERIAL   PRIMARY KEY,
    slug       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE org_idp_configs (
    id           TEXT        NOT NULL PRIMARY KEY,
    org_id       BIGINT      NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    provider     TEXT        NOT NULL,
    display_name TEXT        NOT NULL,
    config       JSONB       NOT NULL,
    sso_required BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE org_members (
    org_id     BIGINT      NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, account_id)
);
CREATE INDEX idx_org_members_account ON org_members(account_id);

CREATE TABLE refresh_tokens (
    id         TEXT        NOT NULL PRIMARY KEY,
    account_id BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    family     TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent TEXT,
    ip_address TEXT
);
CREATE INDEX idx_refresh_tokens_account ON refresh_tokens(account_id);
CREATE INDEX idx_refresh_tokens_family  ON refresh_tokens(family);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);

-- Teams.
CREATE TABLE teams (
    id         BIGSERIAL   PRIMARY KEY,
    org_id     BIGINT      NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    slug       TEXT        NOT NULL,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, slug)
);
CREATE INDEX idx_teams_org ON teams(org_id);

CREATE TABLE team_members (
    team_id    BIGINT      NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    account_id BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, account_id)
);
CREATE INDEX idx_team_members_account ON team_members(account_id);

-- Resources.
CREATE TABLE workspaces (
    id          BIGSERIAL   PRIMARY KEY,
    org_id      BIGINT      REFERENCES organizations(id) ON DELETE CASCADE,
    owner_type  TEXT        NOT NULL,
    owner_id    BIGINT      NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_workspaces_owner ON workspaces(owner_type, owner_id);

CREATE TABLE environments (
    id           BIGSERIAL   PRIMARY KEY,
    workspace_id BIGINT      NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    description  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, name)
);

CREATE TABLE connections (
    id             BIGSERIAL   PRIMARY KEY,
    workspace_id   BIGINT      NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    environment_id BIGINT      NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    name           TEXT        NOT NULL,
    driver         TEXT        NOT NULL,
    dsn_encrypted  TEXT        NOT NULL,
    access_mode    TEXT        NOT NULL DEFAULT 'open',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_connections_workspace ON connections(workspace_id);

-- RBAC policy tables.
CREATE TABLE roles (
    id          BIGSERIAL   PRIMARY KEY,
    org_id      BIGINT      NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    description TEXT,
    scope_type  TEXT        NOT NULL,
    is_builtin  BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, name)
);
CREATE INDEX idx_roles_org ON roles(org_id);

CREATE TABLE role_permissions (
    role_id    BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission TEXT   NOT NULL,
    PRIMARY KEY (role_id, permission)
);

CREATE TABLE role_bindings (
    id            BIGSERIAL   PRIMARY KEY,
    org_id        BIGINT      NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_id       BIGINT      NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    subject_type  TEXT        NOT NULL,
    subject_id    BIGINT      NOT NULL,
    resource_type TEXT        NOT NULL,
    resource_id   BIGINT      NOT NULL,
    expires_at    TIMESTAMPTZ,
    source_type   TEXT,
    source_id     BIGINT,
    created_by    BIGINT      REFERENCES accounts(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(role_id, subject_type, subject_id, resource_type, resource_id)
);
CREATE INDEX idx_role_bindings_resource ON role_bindings(org_id, resource_type, resource_id);
CREATE INDEX idx_role_bindings_subject  ON role_bindings(subject_type, subject_id, org_id);

-- Resource ancestry for transitive permission resolution.
CREATE TABLE resource_hierarchy (
    child_type  TEXT   NOT NULL,
    child_id    BIGINT NOT NULL,
    parent_type TEXT   NOT NULL,
    parent_id   BIGINT NOT NULL,
    owner_type  TEXT   NOT NULL,
    owner_id    BIGINT NOT NULL,
    PRIMARY KEY (child_type, child_id, parent_type, parent_id)
);
CREATE INDEX idx_resource_hierarchy_child ON resource_hierarchy(child_type, child_id);
