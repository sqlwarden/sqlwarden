ALTER TABLE jobs ADD COLUMN singleton_key TEXT;

CREATE UNIQUE INDEX idx_jobs_active_singleton
    ON jobs(singleton_key)
    WHERE singleton_key IS NOT NULL
      AND status IN ('queued', 'running');
