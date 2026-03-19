CREATE TABLE teams (
    id         TEXT    NOT NULL PRIMARY KEY,
    tenant_id  TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug       TEXT    NOT NULL,
    name       TEXT    NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_teams_tenant ON teams(tenant_id);

CREATE TABLE team_members (
    team_id    TEXT    NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    account_id TEXT    NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (team_id, account_id)
);
CREATE INDEX idx_team_members_account ON team_members(account_id);

CREATE TABLE workspaces (
    id          TEXT    NOT NULL PRIMARY KEY,
    tenant_id   TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    description TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workspaces_tenant ON workspaces(tenant_id);
