package settings

import (
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/validator"
)

// MaxDurationSeconds is the largest second count that fits a time.Duration.
const MaxDurationSeconds int64 = 9_223_372_036

// Query history and favorites storage modes.
const (
	HistoryModeBackend = "backend"
	HistoryModeLocal   = "local"
	HistoryModeOff     = "off"
)

// IsSupportedHistoryMode reports whether mode is a query history or favorites
// storage mode.
func IsSupportedHistoryMode(mode string) bool {
	switch mode {
	case HistoryModeBackend, HistoryModeLocal, HistoryModeOff:
		return true
	default:
		return false
	}
}

// Validate checks the invariants every stored instance settings row must
// hold.
func Validate(in Instance) error {
	if strings.TrimSpace(in.InstanceName) == "" {
		return fmt.Errorf("validate runtime settings: instance_name must not be empty")
	}
	if strings.TrimSpace(in.BaseURL) == "" || !validator.IsURL(in.BaseURL) {
		return fmt.Errorf("validate runtime settings: base_url is invalid")
	}
	if in.SupportEmail != "" && !validator.IsEmail(in.SupportEmail) {
		return fmt.Errorf("validate runtime settings: support_email is invalid")
	}
	if in.ErrorNotificationEmail != "" && !validator.IsEmail(in.ErrorNotificationEmail) {
		return fmt.Errorf("validate runtime settings: error_notification_email is invalid")
	}
	if in.JWTAccessTokenTTLSeconds <= 0 || in.JWTAccessTokenTTLSeconds > MaxDurationSeconds {
		return fmt.Errorf("validate runtime settings: jwt_access_token_ttl_seconds is outside the supported range")
	}
	if in.QueryMaxResultRows <= 0 {
		return fmt.Errorf("validate runtime settings: query_max_result_rows must be greater than 0")
	}
	if in.QueryCursorPageSize <= 0 {
		return fmt.Errorf("validate runtime settings: query_cursor_page_size must be greater than 0")
	}
	if in.QueryMaxResultBytes <= 0 {
		return fmt.Errorf("validate runtime settings: query_max_result_bytes must be greater than 0")
	}
	if in.ExportsSyncMaxBytes <= 0 {
		return fmt.Errorf("validate runtime settings: exports_sync_max_bytes must be greater than 0")
	}
	if in.ExportsBackgroundMaxBytes < 0 {
		return fmt.Errorf("validate runtime settings: exports_background_max_bytes must be 0 or greater")
	}
	if in.FileRevisionsKeepLatest < 0 {
		return fmt.Errorf("validate runtime settings: file_revisions_keep_latest must be 0 or greater")
	}
	if !config.IsSupportedLogLevel(in.LogLevel) {
		return fmt.Errorf("validate runtime settings: log_level is unsupported")
	}
	if in.JobsWorkerCount <= 0 || in.JobsWorkerCount > 256 {
		return fmt.Errorf("validate runtime settings: jobs_worker_count is outside the supported range")
	}
	if in.JobsPollIntervalSeconds <= 0 || in.JobsPollIntervalSeconds > 3600 {
		return fmt.Errorf("validate runtime settings: jobs_poll_interval_seconds is outside the supported range")
	}
	if in.JobsClaimLeaseSeconds <= 0 || in.JobsClaimLeaseSeconds > 86400 {
		return fmt.Errorf("validate runtime settings: jobs_claim_lease_seconds is outside the supported range")
	}
	if in.JobsCompletedRetentionSeconds <= 0 || in.JobsCompletedRetentionSeconds > 31536000 {
		return fmt.Errorf("validate runtime settings: jobs_completed_retention_seconds is outside the supported range")
	}
	if in.SMTPPort <= 0 || in.SMTPPort > 65535 {
		return fmt.Errorf("validate runtime settings: smtp_port is outside the supported range")
	}
	if in.SMTPEnabled && strings.TrimSpace(in.SMTPHost) == "" {
		return fmt.Errorf("validate runtime settings: smtp_host is required when SMTP is enabled")
	}
	if in.SMTPEnabled && strings.TrimSpace(in.SMTPFrom) == "" {
		return fmt.Errorf("validate runtime settings: smtp_from is required when SMTP is enabled")
	}
	if !IsSupportedHistoryMode(in.QueryHistoryMode) {
		return fmt.Errorf("validate runtime settings: query_history_mode must be backend, local, or off")
	}
	if !IsSupportedHistoryMode(in.QueryFavoritesMode) {
		return fmt.Errorf("validate runtime settings: query_favorites_mode must be backend, local, or off")
	}
	if in.QueryHistoryRetentionCount < 1 {
		return fmt.Errorf("validate runtime settings: query_history_retention_count must be at least 1")
	}
	if in.QueryHistoryRetentionCountMax < 1 {
		return fmt.Errorf("validate runtime settings: query_history_retention_count_max must be at least 1")
	}
	if in.QueryHistoryRetentionCount > in.QueryHistoryRetentionCountMax {
		return fmt.Errorf("validate runtime settings: query_history_retention_count cannot exceed query_history_retention_count_max")
	}
	return nil
}

// validateOrgOverrides rejects overrides that would widen an instance limit or
// enable a feature the instance turned off.
func validateOrgOverrides(v *validator.Validator, o OrgOverrides, in Instance) {
	if o.QueryMaxResultRows != nil {
		v.CheckField(*o.QueryMaxResultRows > 0 && *o.QueryMaxResultRows <= in.QueryMaxResultRows,
			"query_max_result_rows", "Query row limit must be greater than 0 and no greater than the instance limit.")
	}
	if o.QueryMaxResultBytes != nil {
		v.CheckField(*o.QueryMaxResultBytes > 0 && *o.QueryMaxResultBytes <= in.QueryMaxResultBytes,
			"query_max_result_bytes", "Query byte limit must be greater than 0 and no greater than the instance limit.")
	}
	if o.ExportsSyncMaxBytes != nil {
		v.CheckField(*o.ExportsSyncMaxBytes > 0 && *o.ExportsSyncMaxBytes <= in.ExportsSyncMaxBytes,
			"exports_sync_max_bytes", "Synchronous export limit must be greater than 0 and no greater than the instance limit.")
	}
	if o.ExportsBackgroundMaxBytes != nil {
		valid := *o.ExportsBackgroundMaxBytes >= 0
		if in.ExportsBackgroundMaxBytes > 0 {
			valid = valid && *o.ExportsBackgroundMaxBytes > 0 && *o.ExportsBackgroundMaxBytes <= in.ExportsBackgroundMaxBytes
		}
		v.CheckField(valid, "exports_background_max_bytes", "Background export limit must not exceed the instance limit; 0 is allowed only when the instance is unlimited.")
	}
	if o.FileRevisionsEnabled != nil {
		v.CheckField(!*o.FileRevisionsEnabled || in.FileRevisionsEnabled,
			"file_revisions_enabled", "File revisions cannot be enabled when disabled for the instance.")
	}
	if o.FileRevisionsKeepLatest != nil {
		v.CheckField(*o.FileRevisionsKeepLatest >= 0 && *o.FileRevisionsKeepLatest <= in.FileRevisionsKeepLatest,
			"file_revisions_keep_latest", "Revision retention must be 0 or greater and no greater than the instance limit.")
	}
	if o.QueryHistoryMode != nil {
		v.CheckField(IsSupportedHistoryMode(*o.QueryHistoryMode),
			"query_history_mode", "Query history mode must be backend, local, or off.")
		v.CheckField(in.QueryHistoryMode != HistoryModeOff || *o.QueryHistoryMode == HistoryModeOff,
			"query_history_mode", "Query history cannot be enabled when disabled for the instance.")
	}
	if o.QueryHistoryRetentionCount != nil {
		v.CheckField(*o.QueryHistoryRetentionCount >= 1 && *o.QueryHistoryRetentionCount <= in.QueryHistoryRetentionCount,
			"query_history_retention_count", "Retention count must be at least 1 and no greater than the instance limit.")
	}
	if o.QueryFavoritesMode != nil {
		v.CheckField(IsSupportedHistoryMode(*o.QueryFavoritesMode),
			"query_favorites_mode", "Query favorites mode must be backend, local, or off.")
		v.CheckField(in.QueryFavoritesMode != HistoryModeOff || *o.QueryFavoritesMode == HistoryModeOff,
			"query_favorites_mode", "Query favorites cannot be enabled when disabled for the instance.")
	}
}
