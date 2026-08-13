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
				settings.PersonalSpacesEnabled != want.PersonalSpacesEnabled ||
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
				settings.SMTPEnabled || settings.SMTPPort != want.SMTPPort || settings.SMTPPasswordEncrypted != "" {
				t.Fatalf("unexpected migration defaults: %+v", settings)
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
			settings.PersonalSpacesEnabled = false
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
			if settings.PersonalSpacesEnabled {
				t.Fatal("expected personal spaces to be disabled")
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

			settings, found, err = db.GetInstanceSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !found || settings.PersonalSpacesEnabled {
				t.Fatalf("expected persisted disabled settings, got %+v found=%v", settings, found)
			}

			settings.InstanceName = "Updated Instance"
			settings.InstanceDescription = ""
			settings.SupportEmail = ""
			settings.BaseURL = "https://updated.example.com"
			settings.PersonalSpacesEnabled = true
			settings, err = db.UpsertInstanceSettings(context.Background(), settings)
			if err != nil {
				t.Fatal(err)
			}
			if !settings.PersonalSpacesEnabled {
				t.Fatal("expected personal spaces to be enabled")
			}
			if settings.InstanceName != "Updated Instance" {
				t.Fatalf("expected updated instance name, got %q", settings.InstanceName)
			}
		})
	}
}

func TestListPersonalConnectionIDs(t *testing.T) {
	for _, driver := range testDrivers() {
		driver := driver
		t.Run(driver, func(t *testing.T) {
			t.Parallel()

			db := newTestDB(t, driver)
			ownerID := newAccount(t, db, "owner@example.com")
			org, err := db.InsertOrg(context.Background(), "org-personal-ids", "Org")
			if err != nil {
				t.Fatal(err)
			}

			orgWS, err := db.InsertWorkspace(context.Background(), &org.ID, "org", org.ID, "Org WS", "")
			if err != nil {
				t.Fatal(err)
			}
			spaceWS, err := db.InsertWorkspace(context.Background(), nil, "space", ownerID, "Personal WS", "")
			if err != nil {
				t.Fatal(err)
			}

			orgDefault, err := db.DefaultEnvironmentID(context.Background(), orgWS.ID)
			if err != nil {
				t.Fatal(err)
			}
			spaceDefault, err := db.DefaultEnvironmentID(context.Background(), spaceWS.ID)
			if err != nil {
				t.Fatal(err)
			}

			orgConn, err := db.InsertConnection(context.Background(), orgWS.ID, &orgDefault, "org-db", "sqlite", "enc", "open")
			if err != nil {
				t.Fatal(err)
			}
			spaceConn, err := db.InsertConnection(context.Background(), spaceWS.ID, &spaceDefault, "space-db", "sqlite", "enc", "open")
			if err != nil {
				t.Fatal(err)
			}

			ids, err := db.ListPersonalConnectionIDs(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if containsInt64(ids, orgConn.ID) {
				t.Fatalf("org connection %d should not be returned in personal ids", orgConn.ID)
			}
			if !containsInt64(ids, spaceConn.ID) {
				t.Fatalf("expected personal connection %d in ids %v", spaceConn.ID, ids)
			}
		})
	}
}

func containsInt64(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
