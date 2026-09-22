CREATE TABLE audit_events (
    sequence BIGSERIAL PRIMARY KEY,
    id TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    org_id BIGINT,
    account_id BIGINT,
    action TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure', 'denied')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX idx_audit_events_occurred_at ON audit_events (occurred_at DESC, id DESC);
CREATE INDEX idx_audit_events_org ON audit_events (org_id, occurred_at DESC);
CREATE INDEX idx_audit_events_account ON audit_events (account_id, occurred_at DESC);
CREATE INDEX idx_audit_events_action ON audit_events (action, occurred_at DESC);
