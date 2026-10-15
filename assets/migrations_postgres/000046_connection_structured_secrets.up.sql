ALTER TABLE connections
    ADD COLUMN params JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN tls_config JSONB,
    ADD COLUMN ssh_config JSONB,
    ALTER COLUMN dsn_encrypted DROP NOT NULL;

CREATE TABLE connection_secrets (
    connection_id  BIGINT      NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    name           TEXT        NOT NULL,
    source         TEXT        NOT NULL,
    value_encrypted TEXT       NOT NULL,
    key_id         TEXT        NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (connection_id, name)
);

ALTER TABLE organizations
    DROP COLUMN mask_connection_credentials_on_edit,
    ADD COLUMN allow_connection_secret_reveal BOOLEAN NOT NULL DEFAULT FALSE;

INSERT INTO role_permissions (role_id, permission)
SELECT id, 'conn:reveal_secret'
FROM roles
WHERE workspace_id IS NULL
  AND is_builtin = TRUE
  AND name IN ('Owner', 'Administrator')
ON CONFLICT DO NOTHING;
