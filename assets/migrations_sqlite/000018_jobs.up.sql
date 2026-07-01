CREATE TABLE jobs (
    id                  TEXT     NOT NULL PRIMARY KEY,
    type                TEXT     NOT NULL,
    visibility          TEXT     NOT NULL,
    status              TEXT     NOT NULL,
    org_id              INTEGER  REFERENCES organizations(id) ON DELETE CASCADE,
    workspace_id        INTEGER  REFERENCES workspaces(id) ON DELETE CASCADE,
    owner_account_id    INTEGER  REFERENCES accounts(id) ON DELETE CASCADE,
    run_at              DATETIME NOT NULL,
    priority            INTEGER  NOT NULL DEFAULT 0,
    attempts            INTEGER  NOT NULL DEFAULT 0,
    max_attempts        INTEGER  NOT NULL DEFAULT 1,
    claimed_by          TEXT,
    claimed_until       DATETIME,
    started_at          DATETIME,
    finished_at         DATETIME,
    cancel_requested_at DATETIME,
    error_code          TEXT,
    error_message       TEXT,
    input_json          TEXT,
    output_json         TEXT,
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (visibility IN ('user', 'internal')),
    CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled'))
);

CREATE INDEX idx_jobs_claim
    ON jobs(status, run_at, priority, claimed_until);

CREATE INDEX idx_jobs_workspace_owner
    ON jobs(org_id, workspace_id, owner_account_id, visibility, created_at);

CREATE INDEX idx_jobs_completed
    ON jobs(status, finished_at);
