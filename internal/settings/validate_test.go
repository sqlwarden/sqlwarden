package settings_test

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/settings"
)

func TestValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mutate  func(*database.InstanceSettings)
		wantErr string
	}{
		{name: "canonical row", mutate: func(*database.InstanceSettings) {}},
		{
			name:    "empty instance name",
			mutate:  func(s *database.InstanceSettings) { s.InstanceName = " " },
			wantErr: "instance_name",
		},
		{
			name:    "non-url base url",
			mutate:  func(s *database.InstanceSettings) { s.BaseURL = "not a url" },
			wantErr: "base_url",
		},
		{
			name:    "invalid support email",
			mutate:  func(s *database.InstanceSettings) { s.SupportEmail = "support" },
			wantErr: "support_email",
		},
		{
			name:    "invalid error notification email",
			mutate:  func(s *database.InstanceSettings) { s.ErrorNotificationEmail = "errors" },
			wantErr: "error_notification_email",
		},
		{
			name:    "access token ttl overflows a duration",
			mutate:  func(s *database.InstanceSettings) { s.JWTAccessTokenTTLSeconds = settings.MaxDurationSeconds + 1 },
			wantErr: "jwt_access_token_ttl_seconds",
		},
		{
			name:    "zero cursor page size",
			mutate:  func(s *database.InstanceSettings) { s.QueryCursorPageSize = 0 },
			wantErr: "query_cursor_page_size",
		},
		{
			name:    "negative background export limit",
			mutate:  func(s *database.InstanceSettings) { s.ExportsBackgroundMaxBytes = -1 },
			wantErr: "exports_background_max_bytes",
		},
		{
			name:    "unsupported log level",
			mutate:  func(s *database.InstanceSettings) { s.LogLevel = "verbose" },
			wantErr: "log_level",
		},
		{
			name:    "worker count above the supported range",
			mutate:  func(s *database.InstanceSettings) { s.JobsWorkerCount = 257 },
			wantErr: "jobs_worker_count",
		},
		{
			name:    "smtp port out of range",
			mutate:  func(s *database.InstanceSettings) { s.SMTPPort = 70000 },
			wantErr: "smtp_port",
		},
		{
			name: "smtp enabled without a host",
			mutate: func(s *database.InstanceSettings) {
				s.SMTPEnabled = true
				s.SMTPFrom = "sqlwarden@example.com"
			},
			wantErr: "smtp_host",
		},
		{
			name: "smtp enabled without a sender",
			mutate: func(s *database.InstanceSettings) {
				s.SMTPEnabled = true
				s.SMTPHost = "smtp.example.com"
			},
			wantErr: "smtp_from",
		},
		{
			name:    "unknown query history mode",
			mutate:  func(s *database.InstanceSettings) { s.QueryHistoryMode = "sometimes" },
			wantErr: "query_history_mode",
		},
		{
			name:    "unknown query favorites mode",
			mutate:  func(s *database.InstanceSettings) { s.QueryFavoritesMode = "sometimes" },
			wantErr: "query_favorites_mode",
		},
		{
			name: "history retention above its own maximum",
			mutate: func(s *database.InstanceSettings) {
				s.QueryHistoryRetentionCount = 3000
				s.QueryHistoryRetentionCountMax = 2000
			},
			wantErr: "query_history_retention_count cannot exceed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			instance := validInstanceSettings()
			test.mutate(&instance)

			err := settings.Validate(instance)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want one mentioning %q", err, test.wantErr)
			}
		})
	}
}

func TestIsSupportedHistoryMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{settings.HistoryModeBackend, settings.HistoryModeLocal, settings.HistoryModeOff} {
		if !settings.IsSupportedHistoryMode(mode) {
			t.Fatalf("%q should be supported", mode)
		}
	}
	for _, mode := range []string{"", "Backend", "remote"} {
		if settings.IsSupportedHistoryMode(mode) {
			t.Fatalf("%q should not be supported", mode)
		}
	}
}

func TestValidateOrganizationOverrides(t *testing.T) {
	t.Parallel()
	instance := validInstanceSettings()
	instance.QueryMaxResultRows = 100
	instance.ExportsBackgroundMaxBytes = 1000
	instance.FileRevisionsEnabled = false
	instance.QueryHistoryMode = settings.HistoryModeOff

	tests := []struct {
		name      string
		overrides database.OrganizationRuntimeSettings
		wantField string
	}{
		{
			name:      "valid narrowing overrides",
			overrides: database.OrganizationRuntimeSettings{QueryMaxResultRows: ptr(50)},
		},
		{
			name:      "row limit cannot exceed instance",
			overrides: database.OrganizationRuntimeSettings{QueryMaxResultRows: ptr(101)},
			wantField: "query_max_result_rows",
		},
		{
			name:      "finite background export limit cannot become unlimited",
			overrides: database.OrganizationRuntimeSettings{ExportsBackgroundMaxBytes: ptr(int64(0))},
			wantField: "exports_background_max_bytes",
		},
		{
			name:      "file revisions cannot be enabled",
			overrides: database.OrganizationRuntimeSettings{FileRevisionsEnabled: ptr(true)},
			wantField: "file_revisions_enabled",
		},
		{
			name:      "history cannot be re-enabled",
			overrides: database.OrganizationRuntimeSettings{QueryHistoryMode: ptr(settings.HistoryModeBackend)},
			wantField: "query_history_mode",
		},
		{
			name:      "history mode must be supported",
			overrides: database.OrganizationRuntimeSettings{QueryHistoryMode: ptr("remote")},
			wantField: "query_history_mode",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			violations := settings.ValidateOrganizationOverrides(test.overrides, instance)
			if test.wantField == "" {
				if len(violations) != 0 {
					t.Fatalf("violations = %#v, want none", violations)
				}
				return
			}
			if len(violations) == 0 || violations[0].Field != test.wantField {
				t.Fatalf("violations = %#v, want first field %q", violations, test.wantField)
			}
		})
	}
}
