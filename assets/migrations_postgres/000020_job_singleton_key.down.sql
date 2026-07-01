DROP INDEX IF EXISTS idx_jobs_active_singleton;

ALTER TABLE jobs DROP COLUMN IF EXISTS singleton_key;
