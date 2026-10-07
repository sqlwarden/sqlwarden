package database

import (
	"context"
	"testing"
)

func TestInstanceSettingsMigrationCreatesCanonicalDefaults(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t, driver)
			settings, found, err := db.GetInstanceSettings(context.Background())
			if err != nil || !found {
				t.Fatalf("get settings: found=%v err=%v", found, err)
			}
			want := DefaultInstanceSettings()
			if settings.InstanceName != want.InstanceName ||
				settings.JWTAccessTokenTTLSeconds != want.JWTAccessTokenTTLSeconds ||
				settings.QueryMaxResultRows != want.QueryMaxResultRows ||
				settings.QueryCursorPageSize != want.QueryCursorPageSize ||
				settings.FileRevisionsKeepLatest != want.FileRevisionsKeepLatest ||
				settings.LogLevel != want.LogLevel ||
				settings.DatabaseQueryTracingEnabled != want.DatabaseQueryTracingEnabled ||
				settings.AccessLogsEnabled != want.AccessLogsEnabled ||
				settings.JobsWorkerCount != want.JobsWorkerCount ||
				settings.JobsPollIntervalSeconds != want.JobsPollIntervalSeconds ||
				settings.JobsClaimLeaseSeconds != want.JobsClaimLeaseSeconds ||
				settings.JobsCompletedRetentionSeconds != want.JobsCompletedRetentionSeconds ||
				settings.SMTPEnabled || settings.SMTPPort != want.SMTPPort || settings.SMTPPasswordEncrypted != "" ||
				settings.SQLiteLocalTargetsEnabled != want.SQLiteLocalTargetsEnabled ||
				settings.SQLiteInMemoryTargetsEnabled != want.SQLiteInMemoryTargetsEnabled {
				t.Fatalf("unexpected migration defaults: %+v", settings)
			}
			if !settings.SQLiteLocalTargetsEnabled {
				t.Fatal("expected sqlite local targets to be enabled by default")
			}
			if settings.SQLiteInMemoryTargetsEnabled {
				t.Fatal("expected sqlite in-memory targets to be disabled by default")
			}
		})
	}
}

func TestInitializeInstanceBaseURLOnlySetsEmptyValue(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t, driver)
			ctx := context.Background()

			settings, err := db.InitializeInstanceBaseURL(ctx, "https://first.example.com")
			if err != nil {
				t.Fatal(err)
			}
			if settings.BaseURL != "https://first.example.com" {
				t.Fatalf("base URL = %q", settings.BaseURL)
			}

			settings, err = db.InitializeInstanceBaseURL(ctx, "https://second.example.com")
			if err != nil {
				t.Fatal(err)
			}
			if settings.BaseURL != "https://first.example.com" {
				t.Fatalf("bootstrap URL overwrote runtime value: %q", settings.BaseURL)
			}
		})
	}
}

func TestOrganizationRuntimeSettingsUpsert(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "runtime-settings-"+driver, "Runtime Settings")
			if err != nil {
				t.Fatal(err)
			}
			rows := 100
			revisions := false
			stored, err := db.UpsertOrganizationRuntimeSettings(ctx, OrganizationRuntimeSettings{
				OrgID:                org.ID,
				QueryMaxResultRows:   &rows,
				FileRevisionsEnabled: &revisions,
			})
			if err != nil {
				t.Fatal(err)
			}
			if stored.QueryMaxResultRows == nil || *stored.QueryMaxResultRows != rows || stored.FileRevisionsEnabled == nil || *stored.FileRevisionsEnabled {
				t.Fatalf("unexpected stored overrides: %+v", stored)
			}
			stored.QueryMaxResultRows = nil
			stored, err = db.UpsertOrganizationRuntimeSettings(ctx, stored)
			if err != nil {
				t.Fatal(err)
			}
			if stored.QueryMaxResultRows != nil {
				t.Fatalf("expected cleared override, got %+v", stored)
			}
		})
	}
}

func TestInstanceSettingsUpsert(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()

			db := newTestDB(t, driver)

			settings, found, err := db.GetInstanceSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("expected migration-created instance settings row")
			}

			settings.InstanceName = "Demo Instance"
			settings.InstanceDescription = "A shared SQLWarden instance."
			settings.SupportEmail = "support@example.com"
			settings.BaseURL = "https://sqlwarden.example.com"
			settings.SQLiteLocalTargetsEnabled = false
			settings.SQLiteInMemoryTargetsEnabled = true
			settings.LogLevel = "debug"
			settings.DatabaseQueryTracingEnabled = true
			settings.AccessLogsEnabled = true
			settings.QueryCursorPageSize = 75
			settings.JobsWorkerCount = 4
			settings.SMTPHost = "smtp.example.com"
			settings.SMTPUsername = "mailer"
			settings.SMTPPasswordEncrypted = "encrypted-secret"
			settings.SMTPFrom = "SQLWarden <noreply@example.com>"
			settings, err = db.UpsertInstanceSettings(context.Background(), settings)
			if err != nil {
				t.Fatal(err)
			}
			if settings.InstanceName != "Demo Instance" {
				t.Fatalf("expected instance name to persist, got %q", settings.InstanceName)
			}
			if settings.InstanceDescription != "A shared SQLWarden instance." {
				t.Fatalf("expected instance description to persist, got %q", settings.InstanceDescription)
			}
			if settings.SupportEmail != "support@example.com" {
				t.Fatalf("expected support email to persist, got %q", settings.SupportEmail)
			}
			if settings.BaseURL != "https://sqlwarden.example.com" {
				t.Fatalf("expected base URL to persist, got %q", settings.BaseURL)
			}
			if settings.LogLevel != "debug" || !settings.DatabaseQueryTracingEnabled || !settings.AccessLogsEnabled || settings.QueryCursorPageSize != 75 || settings.JobsWorkerCount != 4 || settings.SMTPPasswordEncrypted != "encrypted-secret" {
				t.Fatalf("expected runtime operations to persist, got %+v", settings)
			}
			if settings.SQLiteLocalTargetsEnabled {
				t.Fatal("expected sqlite local targets to be disabled")
			}
			if !settings.SQLiteInMemoryTargetsEnabled {
				t.Fatal("expected sqlite in-memory targets to be enabled")
			}

			settings, found, err = db.GetInstanceSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !found || settings.SQLiteLocalTargetsEnabled || !settings.SQLiteInMemoryTargetsEnabled {
				t.Fatalf("expected persisted disabled settings, got %+v found=%v", settings, found)
			}

			settings.InstanceName = "Updated Instance"
			settings.InstanceDescription = ""
			settings.SupportEmail = ""
			settings.BaseURL = "https://updated.example.com"
			settings, err = db.UpsertInstanceSettings(context.Background(), settings)
			if err != nil {
				t.Fatal(err)
			}
			if settings.InstanceName != "Updated Instance" {
				t.Fatalf("expected updated instance name, got %q", settings.InstanceName)
			}
		})
	}
}

func TestUpsertInstanceSettings_QueryHistoryFields(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()

			db := newTestDB(t, driver)
			ctx := context.Background()

			settings, found, err := db.GetInstanceSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("expected migration-created instance settings row")
			}

			settings.QueryHistoryMode = "off"
			settings.QueryHistoryRetentionCount = 50
			settings.QueryFavoritesMode = "backend"
			saved, err := db.UpsertInstanceSettings(ctx, settings)
			if err != nil {
				t.Fatal(err)
			}
			if saved.QueryHistoryMode != "off" || saved.QueryHistoryRetentionCount != 50 || saved.QueryFavoritesMode != "backend" {
				t.Fatalf("expected query history fields to persist, got %+v", saved)
			}

			reloaded, found, err := db.GetInstanceSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !found || reloaded.QueryHistoryMode != "off" || reloaded.QueryHistoryRetentionCount != 50 || reloaded.QueryFavoritesMode != "backend" {
				t.Fatalf("expected persisted query history fields, got %+v found=%v", reloaded, found)
			}

			org, err := db.InsertOrg(ctx, "runtime-settings-query-history-"+driver, "Runtime Settings QH")
			if err != nil {
				t.Fatal(err)
			}
			mode := "local"
			count := 25
			stored, err := db.UpsertOrganizationRuntimeSettings(ctx, OrganizationRuntimeSettings{
				OrgID:                      org.ID,
				QueryHistoryMode:           &mode,
				QueryHistoryRetentionCount: &count,
			})
			if err != nil {
				t.Fatal(err)
			}
			if stored.QueryHistoryMode == nil || *stored.QueryHistoryMode != "local" ||
				stored.QueryHistoryRetentionCount == nil || *stored.QueryHistoryRetentionCount != 25 {
				t.Fatalf("expected org override to persist, got %+v", stored)
			}

			stored.QueryHistoryMode = nil
			stored, err = db.UpsertOrganizationRuntimeSettings(ctx, stored)
			if err != nil {
				t.Fatal(err)
			}
			if stored.QueryHistoryMode != nil {
				t.Fatalf("expected cleared override, got %+v", stored)
			}
		})
	}
}
