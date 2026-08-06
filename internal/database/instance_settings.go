package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

const (
	DefaultJWTAccessTokenTTLSeconds       int64 = 86400
	DefaultQueryMaxResultRows                   = 10000
	DefaultQueryMaxResultBytes            int64 = 26214400
	DefaultExportsSyncMaxBytes            int64 = 104857600
	DefaultExportsBackgroundMaxBytes      int64 = 0
	DefaultSchemaSnapshotFreshnessSeconds int64 = 86400
	DefaultFileRevisionsKeepLatest              = 50
	DefaultJobsWorkerCount                      = 16
	DefaultJobsPollIntervalSeconds        int64 = 1
	DefaultJobsClaimLeaseSeconds          int64 = 300
	DefaultJobsCompletedRetentionSeconds  int64 = 604800
	DefaultSMTPPort                             = 25
)

type InstanceSettings struct {
	ID                             int64     `bun:",pk" json:"-"`
	InstanceName                   string    `json:"instance_name"`
	InstanceDescription            string    `json:"instance_description"`
	SupportEmail                   string    `json:"support_email"`
	PublicURL                      string    `json:"public_url"`
	PersonalSpacesEnabled          bool      `bun:",notnull" json:"personal_spaces_enabled"`
	JWTAccessTokenTTLSeconds       int64     `bun:",notnull" json:"jwt_access_token_ttl_seconds"`
	SessionsRevocationEnabled      bool      `bun:",notnull" json:"sessions_revocation_enabled"`
	QueryMaxResultRows             int       `bun:",notnull" json:"query_max_result_rows"`
	QueryMaxResultBytes            int64     `bun:",notnull" json:"query_max_result_bytes"`
	ExportsSyncMaxBytes            int64     `bun:",notnull" json:"exports_sync_max_bytes"`
	ExportsBackgroundMaxBytes      int64     `bun:",notnull" json:"exports_background_max_bytes"`
	SchemaSnapshotFreshnessSeconds int64     `bun:",notnull" json:"schema_snapshot_freshness_seconds"`
	FileRevisionsEnabled           bool      `bun:",notnull" json:"file_revisions_enabled"`
	FileRevisionsKeepLatest        int       `bun:",notnull" json:"file_revisions_keep_latest"`
	ErrorNotificationEmail         string    `bun:",notnull" json:"error_notification_email"`
	LogLevel                       string    `bun:",notnull" json:"log_level"`
	DatabaseQueryTracingEnabled    bool      `bun:",notnull" json:"database_query_tracing_enabled"`
	JobsWorkerCount                int       `bun:",notnull" json:"jobs_worker_count"`
	JobsPollIntervalSeconds        int64     `bun:",notnull" json:"jobs_poll_interval_seconds"`
	JobsClaimLeaseSeconds          int64     `bun:",notnull" json:"jobs_claim_lease_seconds"`
	JobsCompletedRetentionSeconds  int64     `bun:",notnull" json:"jobs_completed_retention_seconds"`
	SMTPEnabled                    bool      `bun:",notnull" json:"smtp_enabled"`
	SMTPHost                       string    `bun:",notnull" json:"smtp_host"`
	SMTPPort                       int       `bun:",notnull" json:"smtp_port"`
	SMTPUsername                   string    `bun:",notnull" json:"smtp_username"`
	SMTPPasswordEncrypted          string    `bun:",notnull" json:"-"`
	SMTPFrom                       string    `bun:",notnull" json:"smtp_from"`
	CreatedAt                      time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt                      time.Time `bun:",notnull" json:"updated_at"`
}

type OrganizationRuntimeSettings struct {
	bun.BaseModel `bun:"table:organization_runtime_settings"`

	OrgID                          int64     `bun:",pk" json:"-"`
	QueryMaxResultRows             *int      `json:"query_max_result_rows"`
	QueryMaxResultBytes            *int64    `json:"query_max_result_bytes"`
	ExportsSyncMaxBytes            *int64    `json:"exports_sync_max_bytes"`
	ExportsBackgroundMaxBytes      *int64    `json:"exports_background_max_bytes"`
	SchemaSnapshotFreshnessSeconds *int64    `json:"schema_snapshot_freshness_seconds"`
	FileRevisionsEnabled           *bool     `json:"file_revisions_enabled"`
	FileRevisionsKeepLatest        *int      `json:"file_revisions_keep_latest"`
	CreatedAt                      time.Time `bun:",notnull" json:"created_at"`
	UpdatedAt                      time.Time `bun:",notnull" json:"updated_at"`
}

func DefaultInstanceSettings() InstanceSettings {
	return InstanceSettings{
		ID:                             1,
		InstanceName:                   "SQLWarden",
		PersonalSpacesEnabled:          true,
		JWTAccessTokenTTLSeconds:       DefaultJWTAccessTokenTTLSeconds,
		SessionsRevocationEnabled:      true,
		QueryMaxResultRows:             DefaultQueryMaxResultRows,
		QueryMaxResultBytes:            DefaultQueryMaxResultBytes,
		ExportsSyncMaxBytes:            DefaultExportsSyncMaxBytes,
		ExportsBackgroundMaxBytes:      DefaultExportsBackgroundMaxBytes,
		SchemaSnapshotFreshnessSeconds: DefaultSchemaSnapshotFreshnessSeconds,
		FileRevisionsEnabled:           true,
		FileRevisionsKeepLatest:        DefaultFileRevisionsKeepLatest,
		LogLevel:                       "info",
		JobsWorkerCount:                DefaultJobsWorkerCount,
		JobsPollIntervalSeconds:        DefaultJobsPollIntervalSeconds,
		JobsClaimLeaseSeconds:          DefaultJobsClaimLeaseSeconds,
		JobsCompletedRetentionSeconds:  DefaultJobsCompletedRetentionSeconds,
		SMTPPort:                       DefaultSMTPPort,
	}
}

func (db *DB) GetInstanceSettings(ctx context.Context) (InstanceSettings, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var settings InstanceSettings
	err := db.NewSelect().Model(&settings).Where("id = 1").Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return InstanceSettings{}, false, nil
	}
	if err != nil {
		return InstanceSettings{}, false, err
	}
	return settings, true, nil
}

func (db *DB) UpsertInstanceSettings(ctx context.Context, settings InstanceSettings) (InstanceSettings, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	settings.ID = 1
	now := time.Now()
	settings.CreatedAt = now
	settings.UpdatedAt = now

	_, err := db.NewInsert().
		Model(&settings).
		On("CONFLICT (id) DO UPDATE").
		Set("instance_name = EXCLUDED.instance_name").
		Set("instance_description = EXCLUDED.instance_description").
		Set("support_email = EXCLUDED.support_email").
		Set("public_url = EXCLUDED.public_url").
		Set("personal_spaces_enabled = EXCLUDED.personal_spaces_enabled").
		Set("jwt_access_token_ttl_seconds = EXCLUDED.jwt_access_token_ttl_seconds").
		Set("sessions_revocation_enabled = EXCLUDED.sessions_revocation_enabled").
		Set("query_max_result_rows = EXCLUDED.query_max_result_rows").
		Set("query_max_result_bytes = EXCLUDED.query_max_result_bytes").
		Set("exports_sync_max_bytes = EXCLUDED.exports_sync_max_bytes").
		Set("exports_background_max_bytes = EXCLUDED.exports_background_max_bytes").
		Set("schema_snapshot_freshness_seconds = EXCLUDED.schema_snapshot_freshness_seconds").
		Set("file_revisions_enabled = EXCLUDED.file_revisions_enabled").
		Set("file_revisions_keep_latest = EXCLUDED.file_revisions_keep_latest").
		Set("error_notification_email = EXCLUDED.error_notification_email").
		Set("log_level = EXCLUDED.log_level").
		Set("database_query_tracing_enabled = EXCLUDED.database_query_tracing_enabled").
		Set("jobs_worker_count = EXCLUDED.jobs_worker_count").
		Set("jobs_poll_interval_seconds = EXCLUDED.jobs_poll_interval_seconds").
		Set("jobs_claim_lease_seconds = EXCLUDED.jobs_claim_lease_seconds").
		Set("jobs_completed_retention_seconds = EXCLUDED.jobs_completed_retention_seconds").
		Set("smtp_enabled = EXCLUDED.smtp_enabled").
		Set("smtp_host = EXCLUDED.smtp_host").
		Set("smtp_port = EXCLUDED.smtp_port").
		Set("smtp_username = EXCLUDED.smtp_username").
		Set("smtp_password_encrypted = EXCLUDED.smtp_password_encrypted").
		Set("smtp_from = EXCLUDED.smtp_from").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return InstanceSettings{}, err
	}

	current, _, err := db.GetInstanceSettings(ctx)
	if err != nil {
		return InstanceSettings{}, err
	}
	return current, nil
}

func (db *DB) GetOrganizationRuntimeSettings(ctx context.Context, orgID int64) (OrganizationRuntimeSettings, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var settings OrganizationRuntimeSettings
	err := db.NewSelect().Model(&settings).Where("org_id = ?", orgID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrganizationRuntimeSettings{OrgID: orgID}, false, nil
	}
	if err != nil {
		return OrganizationRuntimeSettings{}, false, err
	}
	return settings, true, nil
}

func (db *DB) UpsertOrganizationRuntimeSettings(ctx context.Context, settings OrganizationRuntimeSettings) (OrganizationRuntimeSettings, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	now := time.Now()
	settings.CreatedAt = now
	settings.UpdatedAt = now
	_, err := db.NewInsert().Model(&settings).
		On("CONFLICT (org_id) DO UPDATE").
		Set("query_max_result_rows = EXCLUDED.query_max_result_rows").
		Set("query_max_result_bytes = EXCLUDED.query_max_result_bytes").
		Set("exports_sync_max_bytes = EXCLUDED.exports_sync_max_bytes").
		Set("exports_background_max_bytes = EXCLUDED.exports_background_max_bytes").
		Set("schema_snapshot_freshness_seconds = EXCLUDED.schema_snapshot_freshness_seconds").
		Set("file_revisions_enabled = EXCLUDED.file_revisions_enabled").
		Set("file_revisions_keep_latest = EXCLUDED.file_revisions_keep_latest").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return OrganizationRuntimeSettings{}, err
	}
	current, _, err := db.GetOrganizationRuntimeSettings(ctx, settings.OrgID)
	return current, err
}

func (db *DB) ListPersonalConnectionIDs(ctx context.Context) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []int64
	err := db.NewSelect().
		TableExpr("connections c").
		ColumnExpr("c.id").
		Join("JOIN workspaces w ON w.id = c.workspace_id").
		Where("w.owner_type = 'space'").
		Scan(ctx, &ids)
	if err != nil {
		return nil, err
	}
	return ids, nil
}
