package web

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/database"
)

func validInstanceSettingsForTest() database.InstanceSettings {
	settings := database.DefaultInstanceSettings()
	settings.BaseURL = "https://sqlwarden.example.com"
	return settings
}

func TestEffectiveForOrg_QueryHistoryModeCannotLoosenWhenInstanceOff(t *testing.T) {
	t.Parallel()
	app := newTestApplication(t)
	ctx := context.Background()

	org, err := app.db.InsertOrg(ctx, "query-history-off-org", "Query History Off Org")
	if err != nil {
		t.Fatal(err)
	}

	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "off"
	})

	mode := "backend"
	if _, err := app.db.UpsertOrganizationRuntimeSettings(ctx, database.OrganizationRuntimeSettings{
		OrgID:            org.ID,
		QueryHistoryMode: &mode,
	}); err != nil {
		t.Fatal(err)
	}

	effective, err := app.runtimeSettingsService().effectiveForOrg(ctx, &org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effective.QueryHistoryMode != "off" {
		t.Fatalf("expected instance off to win, got %q", effective.QueryHistoryMode)
	}
}

func TestEffectiveForOrg_QueryHistoryModeAppliesOrgOverrideWhenInstanceEnabled(t *testing.T) {
	t.Parallel()
	app := newTestApplication(t)
	ctx := context.Background()

	org, err := app.db.InsertOrg(ctx, "query-history-local-org", "Query History Local Org")
	if err != nil {
		t.Fatal(err)
	}

	mode := "local"
	if _, err := app.db.UpsertOrganizationRuntimeSettings(ctx, database.OrganizationRuntimeSettings{
		OrgID:            org.ID,
		QueryHistoryMode: &mode,
	}); err != nil {
		t.Fatal(err)
	}

	effective, err := app.runtimeSettingsService().effectiveForOrg(ctx, &org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effective.QueryHistoryMode != "local" {
		t.Fatalf("expected org override to apply, got %q", effective.QueryHistoryMode)
	}
}

func TestEffectiveForOrg_QueryHistoryRetentionCountOnlyNarrows(t *testing.T) {
	t.Parallel()
	app := newTestApplication(t)
	ctx := context.Background()

	org, err := app.db.InsertOrg(ctx, "query-history-retention-org", "Query History Retention Org")
	if err != nil {
		t.Fatal(err)
	}

	loosened := database.DefaultQueryHistoryRetentionCount + 100
	if _, err := app.db.UpsertOrganizationRuntimeSettings(ctx, database.OrganizationRuntimeSettings{
		OrgID:                      org.ID,
		QueryHistoryRetentionCount: &loosened,
	}); err != nil {
		t.Fatal(err)
	}
	effective, err := app.runtimeSettingsService().effectiveForOrg(ctx, &org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effective.QueryHistoryRetentionCount != database.DefaultQueryHistoryRetentionCount {
		t.Fatalf("expected org override to be ignored when it loosens the instance default, got %d", effective.QueryHistoryRetentionCount)
	}

	narrowed := 10
	if _, err := app.db.UpsertOrganizationRuntimeSettings(ctx, database.OrganizationRuntimeSettings{
		OrgID:                      org.ID,
		QueryHistoryRetentionCount: &narrowed,
	}); err != nil {
		t.Fatal(err)
	}
	effective, err = app.runtimeSettingsService().effectiveForOrg(ctx, &org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effective.QueryHistoryRetentionCount != narrowed {
		t.Fatalf("expected narrowing org override to apply, got %d", effective.QueryHistoryRetentionCount)
	}
}

func TestValidateInstanceSettings_RejectsRetentionCountAboveMax(t *testing.T) {
	t.Parallel()
	settings := validInstanceSettingsForTest()
	settings.QueryHistoryRetentionCount = 3000
	settings.QueryHistoryRetentionCountMax = 2000

	if err := validateInstanceSettings(settings); err == nil {
		t.Fatal("expected validation error for retention count exceeding max")
	}
}

func TestValidateInstanceSettings_RejectsUnknownQueryHistoryMode(t *testing.T) {
	t.Parallel()
	settings := validInstanceSettingsForTest()
	settings.QueryHistoryMode = "sometimes"

	if err := validateInstanceSettings(settings); err == nil {
		t.Fatal("expected validation error for unsupported query_history_mode")
	}
}
