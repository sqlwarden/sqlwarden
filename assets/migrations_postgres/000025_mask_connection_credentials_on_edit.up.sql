ALTER TABLE organizations
    ADD COLUMN mask_connection_credentials_on_edit BOOLEAN NOT NULL DEFAULT FALSE;
