package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/validator"
)

// ErrValidation is wrapped by every ValidationError.
var ErrValidation = errors.New("settings validation failed")

// ValidationError carries the field and general messages of a rejected
// update. Messages never include submitted values.
type ValidationError struct {
	Validator validator.Validator
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// Patch is a field that distinguishes absent (Set false), explicit null
// (Set true, Value nil), and a value.
type Patch[T any] struct {
	Set   bool
	Value *T
}

func (p *Patch[T]) UnmarshalJSON(data []byte) error {
	p.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		p.Value = nil
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	p.Value = &value
	return nil
}

// UpdateInstanceInput changes the fields that are non-nil. SMTPPassword
// clears the stored password when set to null or empty.
type UpdateInstanceInput struct {
	InstanceName                  *string
	InstanceDescription           *string
	SupportEmail                  *string
	BaseURL                       *string
	JWTAccessTokenTTLSeconds      *int64
	SessionsRevocationEnabled     *bool
	QueryMaxResultRows            *int
	QueryCursorPageSize           *int
	QueryMaxResultBytes           *int64
	ExportsSyncMaxBytes           *int64
	ExportsBackgroundMaxBytes     *int64
	FileRevisionsEnabled          *bool
	FileRevisionsKeepLatest       *int
	ErrorNotificationEmail        *string
	LogLevel                      *string
	DatabaseQueryTracingEnabled   *bool
	AccessLogsEnabled             *bool
	JobsWorkerCount               *int
	JobsPollIntervalSeconds       *int64
	JobsClaimLeaseSeconds         *int64
	JobsCompletedRetentionSeconds *int64
	SMTPEnabled                   *bool
	SMTPHost                      *string
	SMTPPort                      *int
	SMTPUsername                  *string
	SMTPPassword                  Patch[string]
	SMTPFrom                      *string
	QueryHistoryMode              *string
	QueryHistoryRetentionCount    *int
	QueryHistoryRetentionCountMax *int
	QueryFavoritesMode            *string
	SQLiteLocalTargetsEnabled     *bool
	SQLiteInMemoryTargetsEnabled  *bool
}

func (in UpdateInstanceInput) hasPatch() bool {
	return in.InstanceName != nil || in.InstanceDescription != nil || in.SupportEmail != nil ||
		in.BaseURL != nil || in.JWTAccessTokenTTLSeconds != nil || in.SessionsRevocationEnabled != nil ||
		in.QueryMaxResultRows != nil || in.QueryCursorPageSize != nil || in.QueryMaxResultBytes != nil ||
		in.ExportsSyncMaxBytes != nil || in.ExportsBackgroundMaxBytes != nil ||
		in.FileRevisionsEnabled != nil || in.FileRevisionsKeepLatest != nil ||
		in.ErrorNotificationEmail != nil || in.LogLevel != nil ||
		in.DatabaseQueryTracingEnabled != nil || in.AccessLogsEnabled != nil || in.JobsWorkerCount != nil ||
		in.JobsPollIntervalSeconds != nil || in.JobsClaimLeaseSeconds != nil ||
		in.JobsCompletedRetentionSeconds != nil || in.SMTPEnabled != nil ||
		in.SMTPHost != nil || in.SMTPPort != nil || in.SMTPUsername != nil ||
		in.SMTPPassword.Set || in.SMTPFrom != nil ||
		in.QueryHistoryMode != nil || in.QueryHistoryRetentionCount != nil ||
		in.QueryHistoryRetentionCountMax != nil || in.QueryFavoritesMode != nil ||
		in.SQLiteLocalTargetsEnabled != nil || in.SQLiteInMemoryTargetsEnabled != nil
}

// UpdateOrgInput changes the overrides that are Set. A null value clears the
// override so the organization inherits the instance value.
type UpdateOrgInput struct {
	QueryMaxResultRows         Patch[int]
	QueryMaxResultBytes        Patch[int64]
	ExportsSyncMaxBytes        Patch[int64]
	ExportsBackgroundMaxBytes  Patch[int64]
	FileRevisionsEnabled       Patch[bool]
	FileRevisionsKeepLatest    Patch[int]
	QueryHistoryMode           Patch[string]
	QueryHistoryRetentionCount Patch[int]
	QueryFavoritesMode         Patch[string]
}

func (in UpdateOrgInput) hasPatch() bool {
	return in.QueryMaxResultRows.Set || in.QueryMaxResultBytes.Set ||
		in.ExportsSyncMaxBytes.Set || in.ExportsBackgroundMaxBytes.Set ||
		in.FileRevisionsEnabled.Set || in.FileRevisionsKeepLatest.Set ||
		in.QueryHistoryMode.Set || in.QueryHistoryRetentionCount.Set || in.QueryFavoritesMode.Set
}

// OrgView is an organization's stored overrides, the settings in force for
// it, and the instance settings that bound the overrides.
type OrgView struct {
	Overrides OrgOverrides
	Effective Effective
	Instance  Instance
}

// OrgUpdate is the result of UpdateOrg. The RowsRemain flags report stored
// history or favorites left behind when the effective mode stopped being
// backend.
type OrgUpdate struct {
	OrgView
	HistoryRowsRemain   bool
	FavoritesRowsRemain bool
}

// GetInstance returns the instance settings to an instance admin.
func (s *Service) GetInstance(ctx context.Context, p access.Principal) (Instance, error) {
	if err := requireInstanceAdmin(p); err != nil {
		return Instance{}, err
	}
	return s.Instance(ctx)
}

// UpdateInstance validates and stores an instance admin's changes, then
// notifies subscribers with the stored row.
func (s *Service) UpdateInstance(ctx context.Context, p access.Principal, in UpdateInstanceInput) (Instance, error) {
	if err := requireInstanceAdmin(p); err != nil {
		return Instance{}, err
	}
	var v validator.Validator
	v.Check(in.hasPatch(), "At least one setting is required.")
	in.InstanceName = trimmed(in.InstanceName)
	in.InstanceDescription = trimmed(in.InstanceDescription)
	in.SupportEmail = trimmed(in.SupportEmail)
	in.BaseURL = trimmed(in.BaseURL)
	in.ErrorNotificationEmail = trimmed(in.ErrorNotificationEmail)
	in.SMTPHost = trimmed(in.SMTPHost)
	in.SMTPUsername = trimmed(in.SMTPUsername)
	in.SMTPFrom = trimmed(in.SMTPFrom)
	if in.LogLevel != nil {
		level := strings.ToLower(strings.TrimSpace(*in.LogLevel))
		in.LogLevel = &level
	}
	checkInstanceInput(&v, in)
	if v.HasErrors() {
		return Instance{}, &ValidationError{Validator: v}
	}

	current, err := s.Instance(ctx)
	if err != nil {
		return Instance{}, err
	}
	next, err := s.applyInstanceInput(current, in)
	if err != nil {
		return Instance{}, err
	}
	if err := Validate(next); err != nil {
		v.AddError(err.Error())
	}
	if s.fileStorageMode == config.FilesStorageModeFile && next.FileRevisionsEnabled {
		v.AddFieldError("file_revisions_enabled", "File revisions are not supported with file storage mode.")
	}
	if v.HasErrors() {
		return Instance{}, &ValidationError{Validator: v}
	}

	stored, err := s.store.UpsertInstanceSettings(ctx, database.InstanceSettings(next))
	if err != nil {
		return Instance{}, err
	}
	s.publish(Instance(stored))
	return Instance(stored), nil
}

func trimmed(value *string) *string {
	if value == nil {
		return nil
	}
	out := strings.TrimSpace(*value)
	return &out
}

func checkInstanceInput(v *validator.Validator, in UpdateInstanceInput) {
	if in.InstanceName != nil {
		v.CheckField(*in.InstanceName != "", "instance_name", "Instance name is required.")
		v.CheckField(validator.MaxRunes(*in.InstanceName, 120), "instance_name", "Instance name must be 120 characters or fewer.")
	}
	if in.InstanceDescription != nil {
		v.CheckField(validator.MaxRunes(*in.InstanceDescription, 500), "instance_description", "Description must be 500 characters or fewer.")
	}
	if in.SupportEmail != nil {
		v.CheckField(*in.SupportEmail == "" || validator.IsEmail(*in.SupportEmail), "support_email", "Support email must be a valid email address.")
	}
	if in.BaseURL != nil {
		v.CheckField(*in.BaseURL != "" && validator.IsURL(*in.BaseURL), "base_url", "Base URL must be a valid URL.")
	}
	if in.JWTAccessTokenTTLSeconds != nil {
		v.CheckField(*in.JWTAccessTokenTTLSeconds > 0 && *in.JWTAccessTokenTTLSeconds <= MaxDurationSeconds, "jwt_access_token_ttl_seconds", "Access token lifetime is outside the supported range.")
	}
	if in.QueryMaxResultRows != nil {
		v.CheckField(*in.QueryMaxResultRows > 0, "query_max_result_rows", "Query row limit must be greater than 0.")
	}
	if in.QueryCursorPageSize != nil {
		v.CheckField(*in.QueryCursorPageSize > 0, "query_cursor_page_size", "Query cursor page size must be greater than 0.")
	}
	if in.QueryMaxResultBytes != nil {
		v.CheckField(*in.QueryMaxResultBytes > 0, "query_max_result_bytes", "Query byte limit must be greater than 0.")
	}
	if in.ExportsSyncMaxBytes != nil {
		v.CheckField(*in.ExportsSyncMaxBytes > 0, "exports_sync_max_bytes", "Synchronous export limit must be greater than 0.")
	}
	if in.ExportsBackgroundMaxBytes != nil {
		v.CheckField(*in.ExportsBackgroundMaxBytes >= 0, "exports_background_max_bytes", "Background export limit must be 0 or greater.")
	}
	if in.FileRevisionsKeepLatest != nil {
		v.CheckField(*in.FileRevisionsKeepLatest >= 0, "file_revisions_keep_latest", "Revision retention must be 0 or greater.")
	}
	if in.ErrorNotificationEmail != nil {
		v.CheckField(*in.ErrorNotificationEmail == "" || validator.IsEmail(*in.ErrorNotificationEmail), "error_notification_email", "Error notification email must be a valid email address.")
	}
	if in.LogLevel != nil {
		v.CheckField(config.IsSupportedLogLevel(*in.LogLevel), "log_level", "Log level must be debug, info, warn, or error.")
	}
	if in.JobsWorkerCount != nil {
		v.CheckField(*in.JobsWorkerCount > 0 && *in.JobsWorkerCount <= 256, "jobs_worker_count", "Worker count must be between 1 and 256.")
	}
	if in.JobsPollIntervalSeconds != nil {
		v.CheckField(*in.JobsPollIntervalSeconds > 0 && *in.JobsPollIntervalSeconds <= 3600, "jobs_poll_interval_seconds", "Poll interval must be between 1 and 3600 seconds.")
	}
	if in.JobsClaimLeaseSeconds != nil {
		v.CheckField(*in.JobsClaimLeaseSeconds > 0 && *in.JobsClaimLeaseSeconds <= 86400, "jobs_claim_lease_seconds", "Claim lease must be between 1 and 86400 seconds.")
	}
	if in.JobsCompletedRetentionSeconds != nil {
		v.CheckField(*in.JobsCompletedRetentionSeconds > 0 && *in.JobsCompletedRetentionSeconds <= 31536000, "jobs_completed_retention_seconds", "Completed job retention must be between 1 and 31536000 seconds.")
	}
	if in.SMTPHost != nil {
		v.CheckField(validator.MaxRunes(*in.SMTPHost, 253), "smtp_host", "SMTP host must be 253 characters or fewer.")
	}
	if in.SMTPPort != nil {
		v.CheckField(*in.SMTPPort > 0 && *in.SMTPPort <= 65535, "smtp_port", "SMTP port must be between 1 and 65535.")
	}
	if in.SMTPUsername != nil {
		v.CheckField(validator.MaxRunes(*in.SMTPUsername, 320), "smtp_username", "SMTP username must be 320 characters or fewer.")
	}
	if in.SMTPPassword.Value != nil {
		v.CheckField(validator.MaxRunes(*in.SMTPPassword.Value, 4096), "smtp_password", "SMTP password is too long.")
	}
	if in.SMTPFrom != nil {
		v.CheckField(validator.MaxRunes(*in.SMTPFrom, 500), "smtp_from", "SMTP sender must be 500 characters or fewer.")
	}
	if in.QueryHistoryMode != nil {
		v.CheckField(IsSupportedHistoryMode(*in.QueryHistoryMode), "query_history_mode", "Query history mode must be backend, local, or off.")
	}
	if in.QueryHistoryRetentionCount != nil {
		v.CheckField(*in.QueryHistoryRetentionCount >= 1, "query_history_retention_count", "Retention count must be at least 1.")
	}
	if in.QueryHistoryRetentionCountMax != nil {
		v.CheckField(*in.QueryHistoryRetentionCountMax >= 1, "query_history_retention_count_max", "Retention count maximum must be at least 1.")
	}
	if in.QueryFavoritesMode != nil {
		v.CheckField(IsSupportedHistoryMode(*in.QueryFavoritesMode), "query_favorites_mode", "Query favorites mode must be backend, local, or off.")
	}
}

func setIf[T any](dst *T, value *T) {
	if value != nil {
		*dst = *value
	}
}

// applyInstanceInput returns current with in applied. A new SMTP password is
// sealed before it leaves this function and is never returned in plaintext.
func (s *Service) applyInstanceInput(current Instance, in UpdateInstanceInput) (Instance, error) {
	next := current
	setIf(&next.InstanceName, in.InstanceName)
	setIf(&next.InstanceDescription, in.InstanceDescription)
	setIf(&next.SupportEmail, in.SupportEmail)
	setIf(&next.BaseURL, in.BaseURL)
	setIf(&next.JWTAccessTokenTTLSeconds, in.JWTAccessTokenTTLSeconds)
	setIf(&next.SessionsRevocationEnabled, in.SessionsRevocationEnabled)
	setIf(&next.QueryMaxResultRows, in.QueryMaxResultRows)
	setIf(&next.QueryCursorPageSize, in.QueryCursorPageSize)
	setIf(&next.QueryMaxResultBytes, in.QueryMaxResultBytes)
	setIf(&next.ExportsSyncMaxBytes, in.ExportsSyncMaxBytes)
	setIf(&next.ExportsBackgroundMaxBytes, in.ExportsBackgroundMaxBytes)
	setIf(&next.FileRevisionsEnabled, in.FileRevisionsEnabled)
	setIf(&next.FileRevisionsKeepLatest, in.FileRevisionsKeepLatest)
	setIf(&next.ErrorNotificationEmail, in.ErrorNotificationEmail)
	setIf(&next.LogLevel, in.LogLevel)
	setIf(&next.DatabaseQueryTracingEnabled, in.DatabaseQueryTracingEnabled)
	setIf(&next.AccessLogsEnabled, in.AccessLogsEnabled)
	setIf(&next.JobsWorkerCount, in.JobsWorkerCount)
	setIf(&next.JobsPollIntervalSeconds, in.JobsPollIntervalSeconds)
	setIf(&next.JobsClaimLeaseSeconds, in.JobsClaimLeaseSeconds)
	setIf(&next.JobsCompletedRetentionSeconds, in.JobsCompletedRetentionSeconds)
	setIf(&next.SMTPEnabled, in.SMTPEnabled)
	setIf(&next.SMTPHost, in.SMTPHost)
	setIf(&next.SMTPPort, in.SMTPPort)
	setIf(&next.SMTPUsername, in.SMTPUsername)
	setIf(&next.SMTPFrom, in.SMTPFrom)
	if in.SMTPPassword.Set {
		next.SMTPPasswordEncrypted = ""
		if in.SMTPPassword.Value != nil && *in.SMTPPassword.Value != "" {
			if s.sealer == nil {
				return Instance{}, errors.New("settings: secret sealer is not configured")
			}
			sealed, err := s.sealer.Encrypt(*in.SMTPPassword.Value)
			if err != nil {
				return Instance{}, err
			}
			next.SMTPPasswordEncrypted = sealed
		}
	}
	setIf(&next.QueryHistoryMode, in.QueryHistoryMode)
	setIf(&next.QueryHistoryRetentionCount, in.QueryHistoryRetentionCount)
	setIf(&next.QueryHistoryRetentionCountMax, in.QueryHistoryRetentionCountMax)
	setIf(&next.QueryFavoritesMode, in.QueryFavoritesMode)
	setIf(&next.SQLiteLocalTargetsEnabled, in.SQLiteLocalTargetsEnabled)
	setIf(&next.SQLiteInMemoryTargetsEnabled, in.SQLiteInMemoryTargetsEnabled)
	return next, nil
}

// GetOrg returns an organization's runtime settings to a member holding
// org:read.
func (s *Service) GetOrg(ctx context.Context, p access.Principal, orgID int64) (OrgView, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgRead); err != nil {
		return OrgView{}, err
	}
	overrides, _, err := s.store.GetOrganizationRuntimeSettings(ctx, orgID)
	if err != nil {
		return OrgView{}, err
	}
	in, err := s.Instance(ctx)
	if err != nil {
		return OrgView{}, err
	}
	effective, err := s.EffectiveForOrg(ctx, &orgID)
	if err != nil {
		return OrgView{}, err
	}
	return OrgView{Overrides: OrgOverrides(overrides), Effective: effective, Instance: in}, nil
}

// UpdateOrg validates and stores a member's override changes. The member must
// hold org:write.
func (s *Service) UpdateOrg(ctx context.Context, p access.Principal, orgID int64, in UpdateOrgInput) (OrgUpdate, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return OrgUpdate{}, err
	}
	var v validator.Validator
	v.Check(in.hasPatch(), "At least one setting is required.")
	if v.HasErrors() {
		return OrgUpdate{}, &ValidationError{Validator: v}
	}

	row, _, err := s.store.GetOrganizationRuntimeSettings(ctx, orgID)
	if err != nil {
		return OrgUpdate{}, err
	}
	overrides := OrgOverrides(row)
	overrides.OrgID = orgID
	applyPatch(&overrides.QueryMaxResultRows, in.QueryMaxResultRows)
	applyPatch(&overrides.QueryMaxResultBytes, in.QueryMaxResultBytes)
	applyPatch(&overrides.ExportsSyncMaxBytes, in.ExportsSyncMaxBytes)
	applyPatch(&overrides.ExportsBackgroundMaxBytes, in.ExportsBackgroundMaxBytes)
	applyPatch(&overrides.FileRevisionsEnabled, in.FileRevisionsEnabled)
	applyPatch(&overrides.FileRevisionsKeepLatest, in.FileRevisionsKeepLatest)
	applyPatch(&overrides.QueryHistoryMode, in.QueryHistoryMode)
	applyPatch(&overrides.QueryHistoryRetentionCount, in.QueryHistoryRetentionCount)
	applyPatch(&overrides.QueryFavoritesMode, in.QueryFavoritesMode)

	instance, err := s.Instance(ctx)
	if err != nil {
		return OrgUpdate{}, err
	}
	validateOrgOverrides(&v, overrides, instance)
	if v.HasErrors() {
		return OrgUpdate{}, &ValidationError{Validator: v}
	}
	previous, err := s.EffectiveForOrg(ctx, &orgID)
	if err != nil {
		return OrgUpdate{}, err
	}
	stored, err := s.store.UpsertOrganizationRuntimeSettings(ctx, database.OrganizationRuntimeSettings(overrides))
	if err != nil {
		return OrgUpdate{}, err
	}
	effective, err := s.EffectiveForOrg(ctx, &orgID)
	if err != nil {
		return OrgUpdate{}, err
	}
	out := OrgUpdate{OrgView: OrgView{Overrides: OrgOverrides(stored), Effective: effective, Instance: instance}}
	if previous.QueryHistoryMode == HistoryModeBackend && effective.QueryHistoryMode != HistoryModeBackend {
		if out.HistoryRowsRemain, err = s.store.QueryHistoryHasRowsForOrg(ctx, orgID); err != nil {
			return OrgUpdate{}, err
		}
	}
	if previous.QueryFavoritesMode == HistoryModeBackend && effective.QueryFavoritesMode != HistoryModeBackend {
		if out.FavoritesRowsRemain, err = s.store.QueryFavoritesHasRowsForOrg(ctx, orgID); err != nil {
			return OrgUpdate{}, err
		}
	}
	return out, nil
}

func applyPatch[T any](dst **T, patch Patch[T]) {
	if patch.Set {
		*dst = patch.Value
	}
}

// PurgeOrgHistory deletes every backend-stored query history row of the
// organization, typically after its history mode stopped being backend. The
// member must hold org:write.
func (s *Service) PurgeOrgHistory(ctx context.Context, p access.Principal, orgID int64) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return err
	}
	return s.store.ClearQueryHistoryForOrg(ctx, orgID)
}

// PurgeOrgFavorites deletes every backend-stored query favorite of the
// organization. The member must hold org:write.
func (s *Service) PurgeOrgFavorites(ctx context.Context, p access.Principal, orgID int64) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return err
	}
	return s.store.ClearQueryFavoritesForOrg(ctx, orgID)
}
