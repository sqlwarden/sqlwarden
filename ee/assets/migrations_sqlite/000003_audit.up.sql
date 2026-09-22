CREATE TABLE ee_audit_chain_state (
    scope TEXT PRIMARY KEY,
    last_index INTEGER NOT NULL DEFAULT 0,
    last_hash TEXT NOT NULL DEFAULT '',
    last_event_id TEXT NOT NULL DEFAULT '',
    sealed_event_id TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE TABLE ee_audit_chain_records (
    event_id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    chain_index INTEGER NOT NULL,
    previous_hash TEXT NOT NULL,
    hash TEXT NOT NULL,
    signature TEXT NOT NULL DEFAULT '',
    signing_key_id TEXT NOT NULL DEFAULT '',
    retain_until TEXT,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX idx_ee_audit_chain_records_index ON ee_audit_chain_records (scope, chain_index);

CREATE TABLE ee_audit_retention_locks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id INTEGER,
    retain_seconds INTEGER NOT NULL CHECK (retain_seconds > 0),
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX idx_ee_audit_retention_locks_org ON ee_audit_retention_locks (org_id) WHERE org_id IS NOT NULL;
CREATE UNIQUE INDEX idx_ee_audit_retention_locks_instance ON ee_audit_retention_locks ((org_id IS NULL)) WHERE org_id IS NULL;

CREATE TABLE ee_audit_export_cursors (
    exporter TEXT PRIMARY KEY,
    delivered_index INTEGER NOT NULL DEFAULT 0,
    delivered_event_id TEXT NOT NULL DEFAULT '',
    pending_index INTEGER NOT NULL DEFAULT 0,
    pending_event_id TEXT NOT NULL DEFAULT '',
    failure_count INTEGER NOT NULL DEFAULT 0,
    last_error_class TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);
