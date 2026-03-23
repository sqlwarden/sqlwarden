-- Rebuild accounts table without is_superadmin, preserving all original constraints.
-- Original columns from 000003: id, email, name, password, is_active, created_at, updated_at
CREATE TABLE accounts_new (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    password TEXT,
    is_active INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO accounts_new (id, email, name, password, is_active, created_at, updated_at)
    SELECT id, email, name, password, is_active, created_at, updated_at FROM accounts;
DROP TABLE accounts;
ALTER TABLE accounts_new RENAME TO accounts;
CREATE UNIQUE INDEX IF NOT EXISTS accounts_email_idx ON accounts (email);
