CREATE TABLE ee_scim_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_sync_at TEXT,
    cursor TEXT NOT NULL DEFAULT ''
);
