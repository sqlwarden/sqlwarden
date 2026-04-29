PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS workspace_teams;
DROP TABLE IF EXISTS workspace_members;

CREATE TABLE roles_old (
    id          INTEGER  PRIMARY KEY AUTOINCREMENT,
    org_id      INTEGER  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        TEXT     NOT NULL,
    description TEXT,
    scope_type  TEXT     NOT NULL,
    is_builtin  INTEGER  NOT NULL DEFAULT 0,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(org_id, name)
);

INSERT INTO roles_old (id, org_id, name, description, scope_type, is_builtin, created_at, updated_at)
SELECT id, org_id, name, description, scope_type, is_builtin, created_at, updated_at FROM roles WHERE workspace_id IS NULL;

DROP TABLE roles;
ALTER TABLE roles_old RENAME TO roles;

CREATE INDEX idx_roles_org ON roles(org_id);

PRAGMA foreign_keys = ON;
