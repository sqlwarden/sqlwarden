CREATE TABLE auth_sessions (
    id                    TEXT     NOT NULL PRIMARY KEY,
    account_id            INTEGER  NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    user_agent            TEXT,
    ip_address            TEXT,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at            DATETIME NOT NULL,
    revoked_at            DATETIME,
    revoked_by_account_id INTEGER  REFERENCES accounts(id) ON DELETE SET NULL,
    revocation_reason     TEXT
);

CREATE INDEX idx_auth_sessions_account
    ON auth_sessions(account_id, revoked_at, expires_at);

CREATE TABLE org_access_sessions (
    id                    TEXT     NOT NULL PRIMARY KEY,
    auth_session_id       TEXT     NOT NULL REFERENCES auth_sessions(id) ON DELETE CASCADE,
    org_id                INTEGER  NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id            INTEGER  NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at            DATETIME NOT NULL,
    revoked_at            DATETIME,
    revoked_by_account_id INTEGER  REFERENCES accounts(id) ON DELETE SET NULL,
    revocation_reason     TEXT,
    UNIQUE(auth_session_id, org_id)
);

CREATE INDEX idx_org_access_sessions_org_account
    ON org_access_sessions(org_id, account_id, revoked_at, expires_at);

CREATE INDEX idx_org_access_sessions_auth_session
    ON org_access_sessions(auth_session_id);

ALTER TABLE refresh_tokens ADD COLUMN auth_session_id TEXT REFERENCES auth_sessions(id) ON DELETE CASCADE;

CREATE INDEX idx_refresh_tokens_auth_session
    ON refresh_tokens(auth_session_id);
