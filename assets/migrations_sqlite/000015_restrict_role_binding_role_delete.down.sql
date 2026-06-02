PRAGMA foreign_keys = OFF;

CREATE TABLE role_bindings_old (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    org_id        INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_id       INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    subject_type  TEXT    NOT NULL,
    subject_id    INTEGER NOT NULL,
    resource_type TEXT    NOT NULL,
    resource_id   INTEGER NOT NULL,
    expires_at    DATETIME,
    source_type   TEXT,
    source_id     INTEGER,
    created_by    INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(role_id, subject_type, subject_id, resource_type, resource_id)
);

INSERT INTO role_bindings_old (
    id, org_id, role_id, subject_type, subject_id, resource_type, resource_id,
    expires_at, source_type, source_id, created_by, created_at
)
SELECT
    id, org_id, role_id, subject_type, subject_id, resource_type, resource_id,
    expires_at, source_type, source_id, created_by, created_at
FROM role_bindings;

DROP TABLE role_bindings;
ALTER TABLE role_bindings_old RENAME TO role_bindings;

CREATE INDEX idx_role_bindings_resource ON role_bindings(org_id, resource_type, resource_id);
CREATE INDEX idx_role_bindings_subject  ON role_bindings(subject_type, subject_id, org_id);

PRAGMA foreign_keys = ON;
