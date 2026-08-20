ALTER TABLE instance_settings
    ADD COLUMN query_history_mode TEXT NOT NULL DEFAULT 'local'
        CHECK (query_history_mode IN ('backend', 'local', 'off')),
    ADD COLUMN query_history_retention_count INTEGER NOT NULL DEFAULT 500
        CHECK (query_history_retention_count >= 1),
    ADD COLUMN query_history_retention_count_max INTEGER NOT NULL DEFAULT 5000
        CHECK (query_history_retention_count_max >= 1),
    ADD COLUMN query_favorites_mode TEXT NOT NULL DEFAULT 'local'
        CHECK (query_favorites_mode IN ('backend', 'local', 'off'));

ALTER TABLE organization_runtime_settings
    ADD COLUMN query_history_mode TEXT
        CHECK (query_history_mode IS NULL OR query_history_mode IN ('backend', 'local', 'off')),
    ADD COLUMN query_history_retention_count INTEGER
        CHECK (query_history_retention_count IS NULL OR query_history_retention_count >= 1),
    ADD COLUMN query_favorites_mode TEXT
        CHECK (query_favorites_mode IS NULL OR query_favorites_mode IN ('backend', 'local', 'off'));

CREATE TABLE query_history (
    id BIGSERIAL PRIMARY KEY,
    connection_id BIGINT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    sql_text TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'error', 'cancelled')),
    error_message TEXT,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    rows_affected BIGINT NOT NULL DEFAULT 0,
    executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX query_history_connection_account_idx
    ON query_history (connection_id, account_id, executed_at DESC);

CREATE TABLE query_favorites (
    id BIGSERIAL PRIMARY KEY,
    workspace_id BIGINT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    connection_id BIGINT REFERENCES connections(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    sql_text TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX query_favorites_workspace_account_idx
    ON query_favorites (workspace_id, account_id);
