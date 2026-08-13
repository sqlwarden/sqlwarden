ALTER TABLE organizations
    ADD COLUMN mask_connection_credentials_on_edit INTEGER NOT NULL DEFAULT 0;
