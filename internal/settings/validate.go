package settings

import (
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/validator"
)

// MaxDurationSeconds is the largest second count that can be converted to a
// time.Duration without overflowing it.
const MaxDurationSeconds int64 = 9_223_372_036

// Validate reports whether an instance settings row is usable at runtime. It is
// the single source of truth for instance-level settings invariants: startup
// refuses to boot on a row that fails it, reads refuse to serve one, and the
// administration transport checks a candidate row against it before writing.
func Validate(settings database.InstanceSettings) error {
	if strings.TrimSpace(settings.InstanceName) == "" {
		return fmt.Errorf("validate runtime settings: instance_name must not be empty")
	}
	if strings.TrimSpace(settings.BaseURL) == "" || !validator.IsURL(settings.BaseURL) {
		return fmt.Errorf("validate runtime settings: base_url is invalid")
	}
	if settings.SupportEmail != "" && !validator.IsEmail(settings.SupportEmail) {
		return fmt.Errorf("validate runtime settings: support_email is invalid")
	}
	if settings.ErrorNotificationEmail != "" && !validator.IsEmail(settings.ErrorNotificationEmail) {
		return fmt.Errorf("validate runtime settings: error_notification_email is invalid")
	}
	if settings.JWTAccessTokenTTLSeconds <= 0 || settings.JWTAccessTokenTTLSeconds > MaxDurationSeconds {
		return fmt.Errorf("validate runtime settings: jwt_access_token_ttl_seconds is outside the supported range")
	}
	if settings.QueryMaxResultRows <= 0 {
		return fmt.Errorf("validate runtime settings: query_max_result_rows must be greater than 0")
	}
	if settings.QueryCursorPageSize <= 0 {
		return fmt.Errorf("validate runtime settings: query_cursor_page_size must be greater than 0")
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
	if settings.SchemaSnapshotFreshnessSeconds <= 0 || settings.SchemaSnapshotFreshnessSeconds > MaxDurationSeconds {
		return fmt.Errorf("validate runtime settings: schema_snapshot_freshness_seconds is outside the supported range")
	}
	if settings.FileRevisionsKeepLatest < 0 {
		return fmt.Errorf("validate runtime settings: file_revisions_keep_latest must be 0 or greater")
	}
	if !config.IsSupportedLogLevel(settings.LogLevel) {
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
	if !IsSupportedHistoryMode(settings.QueryHistoryMode) {
		return fmt.Errorf("validate runtime settings: query_history_mode must be backend, local, or off")
	}
	if !IsSupportedHistoryMode(settings.QueryFavoritesMode) {
		return fmt.Errorf("validate runtime settings: query_favorites_mode must be backend, local, or off")
	}
	if settings.QueryHistoryRetentionCount < 1 {
		return fmt.Errorf("validate runtime settings: query_history_retention_count must be at least 1")
	}
	if settings.QueryHistoryRetentionCountMax < 1 {
		return fmt.Errorf("validate runtime settings: query_history_retention_count_max must be at least 1")
	}
	if settings.QueryHistoryRetentionCount > settings.QueryHistoryRetentionCountMax {
		return fmt.Errorf("validate runtime settings: query_history_retention_count cannot exceed query_history_retention_count_max")
	}
	return nil
}

// IsSupportedHistoryMode reports whether mode is a supported storage mode for
// query history and query favorites.
func IsSupportedHistoryMode(mode string) bool {
	switch mode {
	case HistoryModeBackend, HistoryModeLocal, HistoryModeOff:
		return true
	default:
		return false
	}
}

// Storage modes for query history and query favorites.
const (
	HistoryModeBackend = "backend"
	HistoryModeLocal   = "local"
	HistoryModeOff     = "off"
)

// Violation identifies one invalid settings field. Transports map violations
// to their own error representation without owning the underlying policy.
type Violation struct {
	Field   string
	Message string
}

// ValidateOrganizationOverrides reports organization overrides that would
// weaken or otherwise violate the instance settings policy.
func ValidateOrganizationOverrides(overrides database.OrganizationRuntimeSettings, instance database.InstanceSettings) []Violation {
	var violations []Violation
	check := func(valid bool, field, message string) {
		if !valid {
			violations = append(violations, Violation{Field: field, Message: message})
		}
	}

	if overrides.QueryMaxResultRows != nil {
		check(*overrides.QueryMaxResultRows > 0 && *overrides.QueryMaxResultRows <= instance.QueryMaxResultRows,
			"query_max_result_rows", "Query row limit must be greater than 0 and no greater than the instance limit.")
	}
	if overrides.QueryMaxResultBytes != nil {
		check(*overrides.QueryMaxResultBytes > 0 && *overrides.QueryMaxResultBytes <= instance.QueryMaxResultBytes,
			"query_max_result_bytes", "Query byte limit must be greater than 0 and no greater than the instance limit.")
	}
	if overrides.ExportsSyncMaxBytes != nil {
		check(*overrides.ExportsSyncMaxBytes > 0 && *overrides.ExportsSyncMaxBytes <= instance.ExportsSyncMaxBytes,
			"exports_sync_max_bytes", "Synchronous export limit must be greater than 0 and no greater than the instance limit.")
	}
	if overrides.ExportsBackgroundMaxBytes != nil {
		valid := *overrides.ExportsBackgroundMaxBytes >= 0
		if instance.ExportsBackgroundMaxBytes > 0 {
			valid = valid && *overrides.ExportsBackgroundMaxBytes > 0 && *overrides.ExportsBackgroundMaxBytes <= instance.ExportsBackgroundMaxBytes
		}
		check(valid, "exports_background_max_bytes", "Background export limit must not exceed the instance limit; 0 is allowed only when the instance is unlimited.")
	}
	if overrides.SchemaSnapshotFreshnessSeconds != nil {
		check(*overrides.SchemaSnapshotFreshnessSeconds >= instance.SchemaSnapshotFreshnessSeconds && *overrides.SchemaSnapshotFreshnessSeconds <= MaxDurationSeconds,
			"schema_snapshot_freshness_seconds", "Schema snapshot freshness must be at least the instance interval.")
	}
	if overrides.FileRevisionsEnabled != nil {
		check(!*overrides.FileRevisionsEnabled || instance.FileRevisionsEnabled,
			"file_revisions_enabled", "File revisions cannot be enabled when disabled for the instance.")
	}
	if overrides.FileRevisionsKeepLatest != nil {
		check(*overrides.FileRevisionsKeepLatest >= 0 && *overrides.FileRevisionsKeepLatest <= instance.FileRevisionsKeepLatest,
			"file_revisions_keep_latest", "Revision retention must be 0 or greater and no greater than the instance limit.")
	}
	if overrides.QueryHistoryMode != nil {
		check(IsSupportedHistoryMode(*overrides.QueryHistoryMode),
			"query_history_mode", "Query history mode must be backend, local, or off.")
		check(instance.QueryHistoryMode != HistoryModeOff || *overrides.QueryHistoryMode == HistoryModeOff,
			"query_history_mode", "Query history cannot be enabled when disabled for the instance.")
	}
	if overrides.QueryHistoryRetentionCount != nil {
		check(*overrides.QueryHistoryRetentionCount >= 1 && *overrides.QueryHistoryRetentionCount <= instance.QueryHistoryRetentionCount,
			"query_history_retention_count", "Retention count must be at least 1 and no greater than the instance limit.")
	}
	if overrides.QueryFavoritesMode != nil {
		check(IsSupportedHistoryMode(*overrides.QueryFavoritesMode),
			"query_favorites_mode", "Query favorites mode must be backend, local, or off.")
		check(instance.QueryFavoritesMode != HistoryModeOff || *overrides.QueryFavoritesMode == HistoryModeOff,
			"query_favorites_mode", "Query favorites cannot be enabled when disabled for the instance.")
	}
	return violations
}
