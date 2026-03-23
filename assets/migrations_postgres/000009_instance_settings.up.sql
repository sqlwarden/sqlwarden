CREATE TABLE instance_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO instance_settings (key, value) VALUES
    ('auth_method', 'password'),
    ('personal_orgs_enabled', 'true'),
    ('sso_enforced', 'false');
