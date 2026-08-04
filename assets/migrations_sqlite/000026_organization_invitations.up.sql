CREATE TABLE organization_invitations (
    id                       TEXT     PRIMARY KEY,
    org_id                   INTEGER  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email                    TEXT     NOT NULL,
    normalized_email         TEXT     NOT NULL,
    invited_by_account_id    INTEGER  REFERENCES accounts(id) ON DELETE SET NULL,
    token_hash               TEXT     NOT NULL UNIQUE,
    expires_at               DATETIME NOT NULL,
    accepted_at              DATETIME,
    accepted_by_account_id   INTEGER  REFERENCES accounts(id) ON DELETE SET NULL,
    revoked_at               DATETIME,
    last_delivery_status     TEXT     NOT NULL DEFAULT 'disabled',
    last_delivery_attempt_at DATETIME,
    created_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (last_delivery_status IN ('sent', 'failed', 'disabled'))
);

CREATE UNIQUE INDEX idx_org_invitations_active_email
    ON organization_invitations(org_id, normalized_email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE INDEX idx_org_invitations_org_created
    ON organization_invitations(org_id, created_at DESC);
