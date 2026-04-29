ALTER TABLE roles ADD COLUMN workspace_id BIGINT REFERENCES workspaces(id) ON DELETE CASCADE;

ALTER TABLE roles DROP CONSTRAINT roles_org_id_name_key;

CREATE UNIQUE INDEX roles_org_name_org_level ON roles(org_id, name)             WHERE workspace_id IS NULL;
CREATE UNIQUE INDEX roles_org_ws_name        ON roles(org_id, workspace_id, name) WHERE workspace_id IS NOT NULL;
CREATE INDEX        idx_roles_workspace      ON roles(workspace_id)              WHERE workspace_id IS NOT NULL;

CREATE TABLE workspace_members (
    workspace_id BIGINT      NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    account_id   BIGINT      NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_by   BIGINT      REFERENCES accounts(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, account_id)
);
CREATE INDEX idx_workspace_members_account ON workspace_members(account_id, workspace_id);

CREATE TABLE workspace_teams (
    workspace_id BIGINT      NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    team_id      BIGINT      NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    created_by   BIGINT      REFERENCES accounts(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (workspace_id, team_id)
);
CREATE INDEX idx_workspace_teams_team ON workspace_teams(team_id, workspace_id);
