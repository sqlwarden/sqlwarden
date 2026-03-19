CREATE TABLE workspace_roles (
    id          TEXT    NOT NULL PRIMARY KEY,
    tenant_id   TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name        TEXT    NOT NULL,
    description TEXT,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name)
);
CREATE INDEX idx_workspace_roles_tenant ON workspace_roles(tenant_id);

CREATE TABLE access_grants (
    id         TEXT    NOT NULL PRIMARY KEY,
    tenant_id  TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subject    TEXT    NOT NULL,
    object     TEXT    NOT NULL,
    action     TEXT    NOT NULL,
    granted_by TEXT    NOT NULL REFERENCES accounts(id),
    expires_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_access_grants_tenant  ON access_grants(tenant_id);
CREATE INDEX idx_access_grants_subject ON access_grants(subject);
CREATE INDEX idx_access_grants_object  ON access_grants(object);
CREATE INDEX idx_access_grants_expires ON access_grants(expires_at) WHERE expires_at IS NOT NULL;
