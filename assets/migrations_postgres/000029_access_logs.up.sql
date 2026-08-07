ALTER TABLE instance_settings
    ADD COLUMN access_logs_enabled BOOLEAN NOT NULL DEFAULT FALSE;
