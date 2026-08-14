CREATE TABLE desktop_installation (
    id         SMALLINT    PRIMARY KEY CHECK (id = 1),
    account_id BIGINT      NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE RESTRICT,
    org_id     BIGINT      NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
