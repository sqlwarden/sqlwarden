CREATE TABLE workspace_file_content_deletions (
    id                 INTEGER  PRIMARY KEY AUTOINCREMENT,
    content_id         INTEGER  NOT NULL REFERENCES workspace_file_contents(id) ON DELETE CASCADE,
    storage_backend_id TEXT     NOT NULL,
    storage_key        TEXT     NOT NULL,
    attempts           INTEGER  NOT NULL DEFAULT 0,
    next_attempt_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_error         TEXT,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(content_id)
);

CREATE INDEX idx_workspace_file_content_deletions_next_attempt
    ON workspace_file_content_deletions(next_attempt_at, id);
