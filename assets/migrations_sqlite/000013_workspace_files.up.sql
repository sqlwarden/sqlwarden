CREATE TABLE workspace_files (
    id                 INTEGER  PRIMARY KEY AUTOINCREMENT,
    workspace_id       INTEGER  NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    parent_id          INTEGER  REFERENCES workspace_files(id) ON DELETE CASCADE,
    visibility         TEXT     NOT NULL CHECK (visibility IN ('private', 'shared')),
    owner_account_id   INTEGER  REFERENCES accounts(id) ON DELETE CASCADE,
    object_type        TEXT     NOT NULL CHECK (object_type IN ('file', 'folder')),
    name               TEXT     NOT NULL,
    media_type         TEXT,
    file_kind          TEXT,
    current_content_id INTEGER,
    created_by         INTEGER  NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    updated_by         INTEGER  NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    deleted_at         DATETIME,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((visibility = 'private' AND owner_account_id IS NOT NULL)
        OR (visibility = 'shared' AND owner_account_id IS NULL))
);
CREATE INDEX idx_workspace_files_workspace ON workspace_files(workspace_id, visibility, owner_account_id, parent_id);
CREATE UNIQUE INDEX workspace_files_private_name_unique
    ON workspace_files(workspace_id, owner_account_id, COALESCE(parent_id, 0), name)
    WHERE visibility = 'private' AND deleted_at IS NULL;
CREATE UNIQUE INDEX workspace_files_shared_name_unique
    ON workspace_files(workspace_id, COALESCE(parent_id, 0), name)
    WHERE visibility = 'shared' AND deleted_at IS NULL;

CREATE TABLE workspace_file_contents (
    id                    INTEGER  PRIMARY KEY AUTOINCREMENT,
    file_id               INTEGER  NOT NULL REFERENCES workspace_files(id) ON DELETE CASCADE,
    version                INTEGER  NOT NULL,
    storage_key            TEXT     NOT NULL,
    content_hash           TEXT     NOT NULL,
    size_bytes             INTEGER  NOT NULL,
    external_modified_at  DATETIME,
    application_encrypted BOOLEAN  NOT NULL DEFAULT FALSE,
    encryption_key_id     TEXT,
    created_by            INTEGER  NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at            DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(file_id, version)
);
CREATE INDEX idx_workspace_file_contents_file ON workspace_file_contents(file_id, version);
