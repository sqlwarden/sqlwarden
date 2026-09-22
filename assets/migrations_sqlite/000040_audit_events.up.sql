CREATE TABLE audit_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    occurred_at TEXT NOT NULL,
    recorded_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    org_id INTEGER,
    account_id INTEGER,
    action TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure', 'denied')),
    metadata TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_audit_events_occurred_at ON audit_events (occurred_at DESC, id DESC);
CREATE INDEX idx_audit_events_org ON audit_events (org_id, occurred_at DESC);
CREATE INDEX idx_audit_events_account ON audit_events (account_id, occurred_at DESC);
CREATE INDEX idx_audit_events_action ON audit_events (action, occurred_at DESC);
