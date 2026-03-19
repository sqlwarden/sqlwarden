CREATE TABLE connections (
    id           TEXT    NOT NULL PRIMARY KEY,
    workspace_id TEXT    NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    tenant_id    TEXT    NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    driver       TEXT    NOT NULL,
    dsn          TEXT    NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_connections_workspace ON connections(workspace_id);
CREATE INDEX idx_connections_tenant    ON connections(tenant_id);
