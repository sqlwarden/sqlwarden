DELETE FROM role_permissions
WHERE permission = 'conn:reveal_secret';

ALTER TABLE organizations
    DROP COLUMN allow_connection_secret_reveal,
    ADD COLUMN mask_connection_credentials_on_edit BOOLEAN NOT NULL DEFAULT FALSE;

DROP TABLE connection_secrets;

UPDATE connections
SET dsn_encrypted = ''
WHERE dsn_encrypted IS NULL;

ALTER TABLE connections
    ALTER COLUMN dsn_encrypted SET NOT NULL,
    DROP COLUMN ssh_config,
    DROP COLUMN tls_config,
    DROP COLUMN params;
