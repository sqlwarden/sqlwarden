package web

import (
	"context"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/database"
	settingsapp "github.com/sqlwarden/internal/settings"
)

// settingsService returns the runtime settings service supplied by the
// composition root. A nil service remains safe for narrowly constructed tests.
func (app *application) settingsService() *settingsapp.Service {
	if app.settings != nil {
		return app.settings
	}
	return settingsapp.New(nil)
}

func (app *application) instanceSettings(ctx context.Context) (database.InstanceSettings, error) {
	return app.settingsService().Instance(ctx)
}

func (app *application) instanceSettingsResponse(settings database.InstanceSettings) map[string]any {
	return map[string]any{
		"instance_name":                     settings.InstanceName,
		"instance_description":              settings.InstanceDescription,
		"support_email":                     settings.SupportEmail,
		"base_url":                          settings.BaseURL,
		"personal_spaces_enabled":           settings.PersonalSpacesEnabled,
		"jwt_access_token_ttl_seconds":      settings.JWTAccessTokenTTLSeconds,
		"sessions_revocation_enabled":       settings.SessionsRevocationEnabled,
		"query_max_result_rows":             settings.QueryMaxResultRows,
		"query_cursor_page_size":            settings.QueryCursorPageSize,
		"query_max_result_bytes":            settings.QueryMaxResultBytes,
		"exports_sync_max_bytes":            settings.ExportsSyncMaxBytes,
		"exports_background_max_bytes":      settings.ExportsBackgroundMaxBytes,
		"schema_snapshot_freshness_seconds": settings.SchemaSnapshotFreshnessSeconds,
		"schema_lazy_threshold":             settings.SchemaLazyThreshold,
		"file_revisions_enabled":            settings.FileRevisionsEnabled,
		"file_revisions_keep_latest":        settings.FileRevisionsKeepLatest,
		"error_notification_email":          settings.ErrorNotificationEmail,
		"log_level":                         settings.LogLevel,
		"database_query_tracing_enabled":    settings.DatabaseQueryTracingEnabled,
		"access_logs_enabled":               settings.AccessLogsEnabled,
		"jobs_worker_count":                 settings.JobsWorkerCount,
		"jobs_poll_interval_seconds":        settings.JobsPollIntervalSeconds,
		"jobs_claim_lease_seconds":          settings.JobsClaimLeaseSeconds,
		"jobs_completed_retention_seconds":  settings.JobsCompletedRetentionSeconds,
		"smtp_enabled":                      settings.SMTPEnabled,
		"smtp_host":                         settings.SMTPHost,
		"smtp_port":                         settings.SMTPPort,
		"smtp_username":                     settings.SMTPUsername,
		"smtp_password_configured":          settings.SMTPPasswordEncrypted != "",
		"smtp_from":                         settings.SMTPFrom,
		"query_history_mode":                settings.QueryHistoryMode,
		"query_history_retention_count":     settings.QueryHistoryRetentionCount,
		"query_history_retention_count_max": settings.QueryHistoryRetentionCountMax,
		"query_favorites_mode":              settings.QueryFavoritesMode,
		"sqlite_local_targets_enabled":      settings.SQLiteLocalTargetsEnabled,
		"sqlite_memory_targets_enabled":     settings.SQLiteInMemoryTargetsEnabled,
	}
}

func (app *application) dropPersonalSpaceSessions(ctx context.Context) error {
	connIDs, err := app.db.ListPersonalConnectionIDs(ctx)
	if err != nil {
		return err
	}
	for _, connID := range connIDs {
		app.connManager.RemoveForConnection(strconv.FormatInt(connID, 10))
	}
	return nil
}

func (app *application) requirePersonalSpacesEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled, err := app.settingsService().PersonalSpacesEnabled(r.Context())
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !enabled {
			app.notFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
