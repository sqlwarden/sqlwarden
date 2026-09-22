CREATE TABLE ee_audit_chain_state (
    scope TEXT PRIMARY KEY,
    last_index BIGINT NOT NULL DEFAULT 0,
    last_hash TEXT NOT NULL DEFAULT '',
    last_event_id TEXT NOT NULL DEFAULT '',
    sealed_event_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ee_audit_chain_records (
    event_id TEXT PRIMARY KEY,
    scope TEXT NOT NULL,
    chain_index BIGINT NOT NULL,
    previous_hash TEXT NOT NULL,
    hash TEXT NOT NULL,
    signature TEXT NOT NULL DEFAULT '',
    signing_key_id TEXT NOT NULL DEFAULT '',
    retain_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_ee_audit_chain_records_index ON ee_audit_chain_records (scope, chain_index);

CREATE TABLE ee_audit_retention_locks (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT,
    retain_seconds BIGINT NOT NULL CHECK (retain_seconds > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_ee_audit_retention_locks_org ON ee_audit_retention_locks (org_id) WHERE org_id IS NOT NULL;
CREATE UNIQUE INDEX idx_ee_audit_retention_locks_instance ON ee_audit_retention_locks ((org_id IS NULL)) WHERE org_id IS NULL;

CREATE TABLE ee_audit_export_cursors (
    exporter TEXT PRIMARY KEY,
    delivered_index BIGINT NOT NULL DEFAULT 0,
    delivered_event_id TEXT NOT NULL DEFAULT '',
    pending_index BIGINT NOT NULL DEFAULT 0,
    pending_event_id TEXT NOT NULL DEFAULT '',
    failure_count BIGINT NOT NULL DEFAULT 0,
    last_error_class TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
