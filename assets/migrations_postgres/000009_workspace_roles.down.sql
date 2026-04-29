DROP TABLE IF EXISTS workspace_teams;
DROP TABLE IF EXISTS workspace_members;

DROP INDEX IF EXISTS idx_roles_workspace;
DROP INDEX IF EXISTS roles_org_ws_name;
DROP INDEX IF EXISTS roles_org_name_org_level;

ALTER TABLE roles ADD CONSTRAINT roles_org_id_name_key UNIQUE (org_id, name);
ALTER TABLE roles DROP COLUMN workspace_id;
