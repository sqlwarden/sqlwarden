package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/validator"
)

var errRuntimeSettingsUnavailable = errors.New("runtime settings unavailable")

const maxRuntimeDurationSeconds int64 = 9_223_372_036

type effectiveRuntimeSettings struct {
	JWTAccessTokenTTL         time.Duration
	SessionsRevocationEnabled bool
	QueryMaxResultRows        int
	QueryMaxResultBytes       int64
	ExportsSyncMaxBytes       int64
	ExportsBackgroundMaxBytes int64
	SchemaSnapshotFreshness   time.Duration
	FileRevisionsEnabled      bool
	FileRevisionsKeepLatest   int
	ErrorNotificationEmail    string
}

type runtimeSettingsService struct {
	db      *database.DB
	baseURL string
}

func newRuntimeSettingsService(db *database.DB, baseURL string) *runtimeSettingsService {
	return &runtimeSettingsService{db: db, baseURL: baseURL}
}

func validateRuntimeSettingsInvariant(ctx context.Context, db *database.DB) error {
	settings, found, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return fmt.Errorf("validate runtime settings: %w", err)
	}
	if !found {
		return fmt.Errorf("validate runtime settings: instance_settings row id=1 is missing; run database migrations")
	}
	return validateInstanceSettings(settings)
}

func validateInstanceSettings(settings database.InstanceSettings) error {
	if strings.TrimSpace(settings.InstanceName) == "" {
		return fmt.Errorf("validate runtime settings: instance_name must not be empty")
	}
	if settings.PublicURL != "" && !validator.IsURL(settings.PublicURL) {
		return fmt.Errorf("validate runtime settings: public_url is invalid")
	}
	if settings.SupportEmail != "" && !validator.IsEmail(settings.SupportEmail) {
		return fmt.Errorf("validate runtime settings: support_email is invalid")
	}
	if settings.ErrorNotificationEmail != "" && !validator.IsEmail(settings.ErrorNotificationEmail) {
		return fmt.Errorf("validate runtime settings: error_notification_email is invalid")
	}
	if settings.JWTAccessTokenTTLSeconds <= 0 || settings.JWTAccessTokenTTLSeconds > maxRuntimeDurationSeconds {
		return fmt.Errorf("validate runtime settings: jwt_access_token_ttl_seconds is outside the supported range")
	}
	if settings.QueryMaxResultRows <= 0 {
		return fmt.Errorf("validate runtime settings: query_max_result_rows must be greater than 0")
	}
	if settings.QueryMaxResultBytes <= 0 {
		return fmt.Errorf("validate runtime settings: query_max_result_bytes must be greater than 0")
	}
	if settings.ExportsSyncMaxBytes <= 0 {
		return fmt.Errorf("validate runtime settings: exports_sync_max_bytes must be greater than 0")
	}
	if settings.ExportsBackgroundMaxBytes < 0 {
		return fmt.Errorf("validate runtime settings: exports_background_max_bytes must be 0 or greater")
	}
	if settings.SchemaSnapshotFreshnessSeconds <= 0 || settings.SchemaSnapshotFreshnessSeconds > maxRuntimeDurationSeconds {
		return fmt.Errorf("validate runtime settings: schema_snapshot_freshness_seconds is outside the supported range")
	}
	if settings.FileRevisionsKeepLatest < 0 {
		return fmt.Errorf("validate runtime settings: file_revisions_keep_latest must be 0 or greater")
	}
	if !isSupportedLogLevel(settings.LogLevel) {
		return fmt.Errorf("validate runtime settings: log_level is unsupported")
	}
	if settings.JobsWorkerCount <= 0 || settings.JobsWorkerCount > 256 {
		return fmt.Errorf("validate runtime settings: jobs_worker_count is outside the supported range")
	}
	if settings.JobsPollIntervalSeconds <= 0 || settings.JobsPollIntervalSeconds > 3600 {
		return fmt.Errorf("validate runtime settings: jobs_poll_interval_seconds is outside the supported range")
	}
	if settings.JobsClaimLeaseSeconds <= 0 || settings.JobsClaimLeaseSeconds > 86400 {
		return fmt.Errorf("validate runtime settings: jobs_claim_lease_seconds is outside the supported range")
	}
	if settings.JobsCompletedRetentionSeconds <= 0 || settings.JobsCompletedRetentionSeconds > 31536000 {
		return fmt.Errorf("validate runtime settings: jobs_completed_retention_seconds is outside the supported range")
	}
	if settings.SMTPPort <= 0 || settings.SMTPPort > 65535 {
		return fmt.Errorf("validate runtime settings: smtp_port is outside the supported range")
	}
	if settings.SMTPEnabled && strings.TrimSpace(settings.SMTPHost) == "" {
		return fmt.Errorf("validate runtime settings: smtp_host is required when SMTP is enabled")
	}
	if settings.SMTPEnabled && strings.TrimSpace(settings.SMTPFrom) == "" {
		return fmt.Errorf("validate runtime settings: smtp_from is required when SMTP is enabled")
	}
	return nil
}

func (s *runtimeSettingsService) instance(ctx context.Context) (database.InstanceSettings, error) {
	if s.db == nil {
		return database.InstanceSettings{}, errRuntimeSettingsUnavailable
	}
	settings, found, err := s.db.GetInstanceSettings(ctx)
	if err != nil {
		return database.InstanceSettings{}, fmt.Errorf("%w: %v", errRuntimeSettingsUnavailable, err)
	}
	if !found {
		return database.InstanceSettings{}, fmt.Errorf("%w: instance settings row is missing", errRuntimeSettingsUnavailable)
	}
	if err := validateInstanceSettings(settings); err != nil {
		return database.InstanceSettings{}, fmt.Errorf("%w: %v", errRuntimeSettingsUnavailable, err)
	}
	if settings.PublicURL == "" {
		settings.PublicURL = s.baseURL
	}
	return settings, nil
}

func effectiveSettingsFromInstance(settings database.InstanceSettings) effectiveRuntimeSettings {
	return effectiveRuntimeSettings{
		JWTAccessTokenTTL:         time.Duration(settings.JWTAccessTokenTTLSeconds) * time.Second,
		SessionsRevocationEnabled: settings.SessionsRevocationEnabled,
		QueryMaxResultRows:        settings.QueryMaxResultRows,
		QueryMaxResultBytes:       settings.QueryMaxResultBytes,
		ExportsSyncMaxBytes:       settings.ExportsSyncMaxBytes,
		ExportsBackgroundMaxBytes: settings.ExportsBackgroundMaxBytes,
		SchemaSnapshotFreshness:   time.Duration(settings.SchemaSnapshotFreshnessSeconds) * time.Second,
		FileRevisionsEnabled:      settings.FileRevisionsEnabled,
		FileRevisionsKeepLatest:   settings.FileRevisionsKeepLatest,
		ErrorNotificationEmail:    settings.ErrorNotificationEmail,
	}
}

func (s *runtimeSettingsService) effectiveForOrg(ctx context.Context, orgID *int64) (effectiveRuntimeSettings, error) {
	instance, err := s.instance(ctx)
	if err != nil {
		return effectiveRuntimeSettings{}, err
	}
	effective := effectiveSettingsFromInstance(instance)
	if orgID == nil {
		return effective, nil
	}
	overrides, found, err := s.db.GetOrganizationRuntimeSettings(ctx, *orgID)
	if err != nil {
		return effectiveRuntimeSettings{}, fmt.Errorf("%w: %v", errRuntimeSettingsUnavailable, err)
	}
	if !found {
		return effective, nil
	}
	if overrides.QueryMaxResultRows != nil {
		if *overrides.QueryMaxResultRows > 0 && *overrides.QueryMaxResultRows < effective.QueryMaxResultRows {
			effective.QueryMaxResultRows = *overrides.QueryMaxResultRows
		}
	}
	if overrides.QueryMaxResultBytes != nil {
		if *overrides.QueryMaxResultBytes > 0 && *overrides.QueryMaxResultBytes < effective.QueryMaxResultBytes {
			effective.QueryMaxResultBytes = *overrides.QueryMaxResultBytes
		}
	}
	if overrides.ExportsSyncMaxBytes != nil {
		if *overrides.ExportsSyncMaxBytes > 0 && *overrides.ExportsSyncMaxBytes < effective.ExportsSyncMaxBytes {
			effective.ExportsSyncMaxBytes = *overrides.ExportsSyncMaxBytes
		}
	}
	if overrides.ExportsBackgroundMaxBytes != nil {
		if effective.ExportsBackgroundMaxBytes == 0 {
			if *overrides.ExportsBackgroundMaxBytes >= 0 {
				effective.ExportsBackgroundMaxBytes = *overrides.ExportsBackgroundMaxBytes
			}
		} else if *overrides.ExportsBackgroundMaxBytes > 0 && *overrides.ExportsBackgroundMaxBytes < effective.ExportsBackgroundMaxBytes {
			effective.ExportsBackgroundMaxBytes = *overrides.ExportsBackgroundMaxBytes
		}
	}
	if overrides.SchemaSnapshotFreshnessSeconds != nil {
		override := time.Duration(*overrides.SchemaSnapshotFreshnessSeconds) * time.Second
		if override > effective.SchemaSnapshotFreshness {
			effective.SchemaSnapshotFreshness = override
		}
	}
	if overrides.FileRevisionsEnabled != nil {
		effective.FileRevisionsEnabled = effective.FileRevisionsEnabled && *overrides.FileRevisionsEnabled
	}
	if overrides.FileRevisionsKeepLatest != nil {
		if *overrides.FileRevisionsKeepLatest >= 0 && *overrides.FileRevisionsKeepLatest < effective.FileRevisionsKeepLatest {
			effective.FileRevisionsKeepLatest = *overrides.FileRevisionsKeepLatest
		}
	}
	return effective, nil
}

func (app *application) runtimeSettingsService() *runtimeSettingsService {
	if app.runtimeSettings != nil {
		return app.runtimeSettings
	}
	return newRuntimeSettingsService(app.db, app.config.BaseURL)
}

func (app *application) effectiveRuntimeSettingsForWorkspace(ctx context.Context, workspace database.Workspace) (effectiveRuntimeSettings, error) {
	if workspace.OwnerType == "org" {
		orgID := workspace.OwnerID
		return app.runtimeSettingsService().effectiveForOrg(ctx, &orgID)
	}
	return app.runtimeSettingsService().effectiveForOrg(ctx, nil)
}

func (app *application) personalSpacesEnabled(ctx context.Context) (bool, error) {
	settings, err := app.instanceSettings(ctx)
	if err != nil {
		return false, err
	}
	return settings.PersonalSpacesEnabled, nil
}

func (app *application) instanceSettings(ctx context.Context) (database.InstanceSettings, error) {
	return app.runtimeSettingsService().instance(ctx)
}

func (app *application) instanceSettingsResponse(settings database.InstanceSettings) map[string]any {
	return map[string]any{
		"instance_name":                     settings.InstanceName,
		"instance_description":              settings.InstanceDescription,
		"support_email":                     settings.SupportEmail,
		"public_url":                        settings.PublicURL,
		"personal_spaces_enabled":           settings.PersonalSpacesEnabled,
		"jwt_access_token_ttl_seconds":      settings.JWTAccessTokenTTLSeconds,
		"sessions_revocation_enabled":       settings.SessionsRevocationEnabled,
		"query_max_result_rows":             settings.QueryMaxResultRows,
		"query_max_result_bytes":            settings.QueryMaxResultBytes,
		"exports_sync_max_bytes":            settings.ExportsSyncMaxBytes,
		"exports_background_max_bytes":      settings.ExportsBackgroundMaxBytes,
		"schema_snapshot_freshness_seconds": settings.SchemaSnapshotFreshnessSeconds,
		"file_revisions_enabled":            settings.FileRevisionsEnabled,
		"file_revisions_keep_latest":        settings.FileRevisionsKeepLatest,
		"error_notification_email":          settings.ErrorNotificationEmail,
		"log_level":                         settings.LogLevel,
		"database_query_tracing_enabled":    settings.DatabaseQueryTracingEnabled,
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
		enabled, err := app.personalSpacesEnabled(r.Context())
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
