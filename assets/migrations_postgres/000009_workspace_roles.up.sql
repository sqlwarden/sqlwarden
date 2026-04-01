ALTER TABLE roles ADD COLUMN workspace_id BIGINT REFERENCES workspaces(id) ON DELETE CASCADE;

ALTER TABLE roles DROP CONSTRAINT roles_org_id_name_key;

CREATE UNIQUE INDEX roles_org_name_org_level ON roles(org_id, name)             WHERE workspace_id IS NULL;
CREATE UNIQUE INDEX roles_org_ws_name        ON roles(org_id, workspace_id, name) WHERE workspace_id IS NOT NULL;
CREATE INDEX        idx_roles_workspace      ON roles(workspace_id)              WHERE workspace_id IS NOT NULL;
