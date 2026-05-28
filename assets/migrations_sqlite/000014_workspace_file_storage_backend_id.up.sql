ALTER TABLE workspace_file_contents
ADD COLUMN storage_backend_id TEXT NOT NULL DEFAULT 'local';

CREATE INDEX idx_workspace_file_contents_storage_backend
    ON workspace_file_contents(storage_backend_id);
