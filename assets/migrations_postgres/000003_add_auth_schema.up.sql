CREATE TABLE accounts (
    id         TEXT        NOT NULL PRIMARY KEY,
    email      TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    password   TEXT,
    is_active  BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenants (
    id         TEXT        NOT NULL PRIMARY KEY,
    slug       TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenant_idp_configs (
    id           TEXT        NOT NULL PRIMARY KEY,
    tenant_id    TEXT        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider     TEXT        NOT NULL,
    display_name TEXT        NOT NULL,
    config       JSONB       NOT NULL,
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id)
);
CREATE INDEX idx_tenant_idp_tenant ON tenant_idp_configs(tenant_id);

CREATE TABLE tenant_members (
    tenant_id  TEXT        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    account_id TEXT        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role       TEXT        NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, account_id)
);
CREATE INDEX idx_tenant_members_account ON tenant_members(account_id);

CREATE TABLE refresh_tokens (
    id         TEXT        NOT NULL PRIMARY KEY,
    account_id TEXT        NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash TEXT        NOT NULL UNIQUE,
    family     TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent TEXT,
    ip_address TEXT
);
CREATE INDEX idx_refresh_tokens_account ON refresh_tokens(account_id);
CREATE INDEX idx_refresh_tokens_family  ON refresh_tokens(family);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);
