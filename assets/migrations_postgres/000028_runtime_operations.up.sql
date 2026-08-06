ALTER TABLE instance_settings
    ADD COLUMN log_level TEXT NOT NULL DEFAULT 'info' CHECK (log_level IN ('debug', 'info', 'warn', 'error')),
    ADD COLUMN database_query_tracing_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN jobs_worker_count INTEGER NOT NULL DEFAULT 16 CHECK (jobs_worker_count > 0 AND jobs_worker_count <= 256),
    ADD COLUMN jobs_poll_interval_seconds BIGINT NOT NULL DEFAULT 1 CHECK (jobs_poll_interval_seconds > 0 AND jobs_poll_interval_seconds <= 3600),
    ADD COLUMN jobs_claim_lease_seconds BIGINT NOT NULL DEFAULT 300 CHECK (jobs_claim_lease_seconds > 0 AND jobs_claim_lease_seconds <= 86400),
    ADD COLUMN jobs_completed_retention_seconds BIGINT NOT NULL DEFAULT 604800 CHECK (jobs_completed_retention_seconds > 0 AND jobs_completed_retention_seconds <= 31536000),
    ADD COLUMN smtp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN smtp_host TEXT NOT NULL DEFAULT '',
    ADD COLUMN smtp_port INTEGER NOT NULL DEFAULT 25 CHECK (smtp_port > 0 AND smtp_port <= 65535),
    ADD COLUMN smtp_username TEXT NOT NULL DEFAULT '',
    ADD COLUMN smtp_password_encrypted TEXT NOT NULL DEFAULT '',
    ADD COLUMN smtp_from TEXT NOT NULL DEFAULT '';
