CREATE TABLE ee_scim_state (
    id BIGINT PRIMARY KEY CHECK (id = 1),
    last_sync_at TIMESTAMPTZ,
    cursor TEXT NOT NULL DEFAULT ''
);
