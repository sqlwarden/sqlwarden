ALTER TABLE auth_sessions ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'password';
ALTER TABLE auth_sessions ADD COLUMN assurance TEXT NOT NULL DEFAULT 'aal1';
