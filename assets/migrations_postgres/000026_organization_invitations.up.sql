CREATE TABLE organization_invitations (
    id                       TEXT        PRIMARY KEY,
    org_id                   BIGINT      NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email                    TEXT        NOT NULL,
    normalized_email         TEXT        NOT NULL,
    invited_by_account_id    BIGINT      REFERENCES accounts(id) ON DELETE SET NULL,
    token_hash               TEXT        NOT NULL UNIQUE,
    expires_at               TIMESTAMPTZ NOT NULL,
    accepted_at              TIMESTAMPTZ,
    accepted_by_account_id   BIGINT      REFERENCES accounts(id) ON DELETE SET NULL,
    revoked_at               TIMESTAMPTZ,
    last_delivery_status     TEXT        NOT NULL DEFAULT 'disabled',
    last_delivery_attempt_at TIMESTAMPTZ,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (last_delivery_status IN ('sent', 'failed', 'disabled'))
);

CREATE UNIQUE INDEX idx_org_invitations_active_email
    ON organization_invitations(org_id, normalized_email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_org_invitations_org_created
    ON organization_invitations(org_id, created_at DESC);
