CREATE TABLE ee_access_deny_rules (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL,
    account_id BIGINT,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id BIGINT NOT NULL DEFAULT 0,
    permission TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ee_access_deny_rules_lookup ON ee_access_deny_rules (org_id, permission);

CREATE TABLE ee_conditional_access_rules (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL,
    permission TEXT NOT NULL,
    required_auth_method TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ee_conditional_access_rules_lookup ON ee_conditional_access_rules (org_id, permission);

CREATE TABLE ee_jit_access_policies (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id BIGINT NOT NULL DEFAULT 0,
    permission TEXT NOT NULL,
    max_duration_seconds INTEGER NOT NULL DEFAULT 3600,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ee_jit_access_policies_lookup ON ee_jit_access_policies (org_id, permission);

CREATE TABLE ee_jit_activations (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id BIGINT NOT NULL DEFAULT 0,
    permission TEXT NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_ee_jit_activations_lookup ON ee_jit_activations (org_id, account_id, permission);

CREATE TABLE ee_directory_identities (
    id BIGSERIAL PRIMARY KEY,
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL,
    email TEXT NOT NULL,
    account_id BIGINT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_ee_directory_identities_external ON ee_directory_identities (provider, external_id);
CREATE UNIQUE INDEX idx_ee_directory_identities_email ON ee_directory_identities (provider, email);
