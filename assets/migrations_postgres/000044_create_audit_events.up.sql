CREATE TABLE audit_events (
    sequence BIGSERIAL PRIMARY KEY,
    id TEXT NOT NULL UNIQUE,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    org_id BIGINT,
    subject_kind TEXT NOT NULL DEFAULT '',
    subject_id BIGINT,
    on_behalf_of_kind TEXT NOT NULL DEFAULT '',
    on_behalf_of_id BIGINT,
    credential_kind TEXT NOT NULL DEFAULT '',
    credential_id TEXT NOT NULL DEFAULT '',
    client_id TEXT NOT NULL DEFAULT '',
    auth_method TEXT NOT NULL DEFAULT '',
    assurance TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    resource TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure', 'denied')),
    decision_reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX audit_events_occurred_idx ON audit_events (occurred_at DESC, id DESC);
CREATE INDEX audit_events_org_idx ON audit_events (org_id, occurred_at DESC);
CREATE INDEX audit_events_subject_idx ON audit_events (subject_kind, subject_id, occurred_at DESC);
CREATE INDEX audit_events_action_idx ON audit_events (action, occurred_at DESC);
