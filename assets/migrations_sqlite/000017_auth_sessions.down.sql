DROP INDEX IF EXISTS idx_refresh_tokens_auth_session;
DROP INDEX IF EXISTS idx_org_access_sessions_auth_session;
DROP INDEX IF EXISTS idx_org_access_sessions_org_account;
DROP INDEX IF EXISTS idx_auth_sessions_account;

ALTER TABLE refresh_tokens DROP COLUMN auth_session_id;

DROP TABLE IF EXISTS org_access_sessions;
DROP TABLE IF EXISTS auth_sessions;
