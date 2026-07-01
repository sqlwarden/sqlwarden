CREATE TABLE job_events (
    id           TEXT        NOT NULL PRIMARY KEY,
    job_id       TEXT        NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    level        TEXT        NOT NULL,
    code         TEXT        NOT NULL,
    message      TEXT        NOT NULL,
    details_json TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (level IN ('info', 'warn', 'error'))
);

CREATE INDEX idx_job_events_job
    ON job_events(job_id, id);
