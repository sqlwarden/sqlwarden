package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/pascaldekloe/jwt"
	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func TestInstanceRuntimeSettingsAPI(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "runtime-admin@example.com", "Runtime Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{
		"jwt_access_token_ttl_seconds":      1800,
		"sessions_revocation_enabled":       false,
		"query_max_result_rows":             2000,
		"query_cursor_page_size":            250,
		"query_max_result_bytes":            4096,
		"exports_sync_max_bytes":            8192,
		"exports_background_max_bytes":      0,
		"schema_snapshot_freshness_seconds": 7200,
		"file_revisions_enabled":            true,
		"file_revisions_keep_latest":        10,
		"error_notification_email":          "errors@example.com",
	}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["jwt_access_token_ttl_seconds"], any(float64(1800)))
	assert.Equal(t, res.BodyFields["query_max_result_rows"], any(float64(2000)))
	assert.Equal(t, res.BodyFields["query_cursor_page_size"], any(float64(250)))
	assert.Equal(t, res.BodyFields["error_notification_email"], "errors@example.com")

	settings, err := app.runtimeSettingsService().effectiveForOrg(context.Background(), nil)
	assert.Nil(t, err)
	assert.Equal(t, settings.QueryMaxResultRows, 2000)
	assert.Equal(t, settings.QueryCursorPageSize, 250)
	assert.Equal(t, settings.QueryMaxResultBytes, int64(4096))
	assert.Equal(t, settings.SessionsRevocationEnabled, false)

	register := registerTestUser(t, app, "runtime-user@example.com", "Runtime User", "securepass99")
	assert.Equal(t, register.StatusCode, http.StatusCreated)
	login := loginTestUser(t, app, "runtime-user@example.com", "securepass99")
	assert.Equal(t, login.StatusCode, http.StatusOK)
	claims, err := jwt.HMACCheck([]byte(extractAccessToken(t, login)), []byte(app.config.JWT.SecretKey))
	assert.Nil(t, err)
	remaining := time.Until(claims.Expires.Time())
	if remaining < 29*time.Minute || remaining > 31*time.Minute {
		t.Fatalf("new access token lifetime = %s, want approximately 30m", remaining)
	}
}

func TestInstanceRuntimeOperationsAndWriteOnlySMTPPassword(t *testing.T) {
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "runtime-operations@example.com", "Runtime Operations", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{
		"log_level": "debug", "database_query_tracing_enabled": true, "access_logs_enabled": true,
		"jobs_worker_count": 3, "jobs_poll_interval_seconds": 2,
		"jobs_claim_lease_seconds": 90, "jobs_completed_retention_seconds": 86400,
		"smtp_enabled": true, "smtp_host": "smtp.example.com", "smtp_port": 587,
		"smtp_username": "mailer@example.com", "smtp_password": "top-secret-password",
		"smtp_from": "SQLWarden <noreply@example.com>",
	}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["log_level"], "debug")
	assert.Equal(t, res.BodyFields["database_query_tracing_enabled"], true)
	assert.Equal(t, res.BodyFields["access_logs_enabled"], true)
	assert.Equal(t, res.BodyFields["smtp_password_configured"], true)
	if _, ok := res.BodyFields["smtp_password"]; ok {
		t.Fatal("runtime settings response exposed SMTP password")
	}
	if _, ok := res.BodyFields["smtp_password_encrypted"]; ok {
		t.Fatal("runtime settings response exposed encrypted SMTP password")
	}

	stored, found, err := app.db.GetInstanceSettings(context.Background())
	if err != nil || !found {
		t.Fatalf("get stored settings: found=%v err=%v", found, err)
	}
	if stored.SMTPPasswordEncrypted == "" || stored.SMTPPasswordEncrypted == "top-secret-password" {
		t.Fatalf("SMTP password was not encrypted at rest: %q", stored.SMTPPasswordEncrypted)
	}
	if !stored.AccessLogsEnabled {
		t.Fatal("access log setting was not persisted")
	}
	if err := app.applyRuntimeOperations(stored); err != nil {
		t.Fatal(err)
	}
	if !app.accessLogsEnabled.Load() {
		t.Fatal("access log setting was not applied to the running instance")
	}
	plaintext, err := app.keyring.Decrypt(stored.SMTPPasswordEncrypted)
	assert.Nil(t, err)
	assert.Equal(t, plaintext, "top-secret-password")

	res = send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{"smtp_username": "updated@example.com"}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	storedAfterOmit, _, err := app.db.GetInstanceSettings(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, storedAfterOmit.SMTPPasswordEncrypted, stored.SMTPPasswordEncrypted)

	res = send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{"smtp_password": nil}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["smtp_password_configured"], false)
	storedAfterClear, _, err := app.db.GetInstanceSettings(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, storedAfterClear.SMTPPasswordEncrypted, "")
}

func TestInstanceRuntimeOperationsValidationIsAtomic(t *testing.T) {
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "runtime-operations-validation@example.com", "Runtime Operations", "securepass99")
	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{
		"log_level": "verbose", "jobs_worker_count": 0, "smtp_port": 70000,
	}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "log_level")
	assertValidationField(t, res, "jobs_worker_count")
	assertValidationField(t, res, "smtp_port")
	settings, err := app.instanceSettings(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, settings.LogLevel, LogLevelInfo)
}

func TestInstanceRuntimeSettingsValidationIsAtomic(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "runtime-validation@example.com", "Runtime Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{
		"query_max_result_rows":             0,
		"query_cursor_page_size":            0,
		"schema_snapshot_freshness_seconds": -1,
		"error_notification_email":          "invalid",
	}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "query_max_result_rows")
	assertValidationField(t, res, "query_cursor_page_size")
	assertValidationField(t, res, "schema_snapshot_freshness_seconds")
	assertValidationField(t, res, "error_notification_email")

	settings, err := app.instanceSettings(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, settings.QueryMaxResultRows, database.DefaultQueryMaxResultRows)
	assert.Equal(t, settings.QueryCursorPageSize, database.DefaultQueryCursorPageSize)
}

func TestInstanceBootstrapConfigurationIsSanitized(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "configuration-admin@example.com", "Configuration Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/configuration", nil, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["deployment_managed"], true)
	assert.Equal(t, res.BodyFields["restart_required"], true)
	if _, exists := res.BodyFields["base_url"]; exists {
		t.Fatal("bootstrap response exposed base_url")
	}
	for _, forbidden := range []string{"database_dsn", "cookie_secret_key", "jwt_secret_key", "encryption_key", "tls_key_file", "files_root_dir"} {
		if _, exists := res.BodyFields[forbidden]; exists {
			t.Fatalf("bootstrap response exposed %q", forbidden)
		}
	}
}

func TestRuntimeSettingsReadFailureReturnsServiceUnavailable(t *testing.T) {
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "settings-failure@example.com", "Settings Failure", "securepass99")
	if err := app.db.Close(); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/settings", nil, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusServiceUnavailable)
	assertAPIError(t, res, apiErrorSettingsUnavailable, "Runtime settings are temporarily unavailable.")
}

func TestConfiguredInstanceWithMissingBaseURLFailsInvariantInsteadOfRebootstrapping(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "base-url-invariant@example.com", "Base URL Invariant", "securepass99")
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.BaseURL = ""
	})

	err := initializeInstanceBaseURL(context.Background(), app.db, "https://bootstrap.example.com")
	if err == nil || !strings.Contains(err.Error(), "base_url is invalid") {
		t.Fatalf("initialize base URL error = %v", err)
	}
}

func TestRuntimeSettingsMissingRowReturnsServiceUnavailable(t *testing.T) {
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "settings-missing@example.com", "Settings Missing", "securepass99")
	if _, err := app.db.ExecContext(context.Background(), "DELETE FROM instance_settings WHERE id = 1"); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/settings", nil, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusServiceUnavailable)
	assertAPIError(t, res, apiErrorSettingsUnavailable, "Runtime settings are temporarily unavailable.")
}

func TestRuntimeSettingsInvalidRowReturnsServiceUnavailable(t *testing.T) {
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "settings-invalid@example.com", "Settings Invalid", "securepass99")
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.InstanceName = ""
	})

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/settings", nil, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusServiceUnavailable)
	assertAPIError(t, res, apiErrorSettingsUnavailable, "Runtime settings are temporarily unavailable.")
}

func TestValidateRuntimeSettingsInvariant(t *testing.T) {
	t.Run("accepts canonical migration row", func(t *testing.T) {
		app := newTestApp(t)
		if err := validateRuntimeSettingsInvariant(context.Background(), app.db); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects missing singleton row", func(t *testing.T) {
		app := newTestApp(t)
		if _, err := app.db.ExecContext(context.Background(), "DELETE FROM instance_settings WHERE id = 1"); err != nil {
			t.Fatal(err)
		}
		err := validateRuntimeSettingsInvariant(context.Background(), app.db)
		if err == nil || !strings.Contains(err.Error(), "row id=1 is missing") {
			t.Fatalf("unexpected invariant error: %v", err)
		}
	})

	t.Run("rejects invalid singleton values", func(t *testing.T) {
		app := newTestApp(t)
		updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
			settings.InstanceName = ""
		})
		err := validateRuntimeSettingsInvariant(context.Background(), app.db)
		if err == nil || !strings.Contains(err.Error(), "instance_name must not be empty") {
			t.Fatalf("unexpected invariant error: %v", err)
		}
	})

	t.Run("rejects an invalid cursor page size", func(t *testing.T) {
		app := newTestApp(t)
		updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
			settings.QueryCursorPageSize = 0
		})
		err := validateRuntimeSettingsInvariant(context.Background(), app.db)
		if err == nil || !strings.Contains(err.Error(), "query_cursor_page_size") {
			t.Fatalf("unexpected invariant error: %v", err)
		}
	})

}

func TestOrganizationRuntimeSettingsInheritanceAndClear(t *testing.T) {
	t.Parallel()
	app, org, _, token := setupWorkspaceOwner(t)

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/runtime-settings", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	effective := res.BodyFields["effective"].(map[string]any)
	assert.Equal(t, effective["query_max_result_rows"], any(float64(database.DefaultQueryMaxResultRows)))

	res = send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug+"/runtime-settings", map[string]any{
		"query_max_result_rows":             100,
		"query_max_result_bytes":            1024,
		"exports_sync_max_bytes":            4096,
		"schema_snapshot_freshness_seconds": 172800,
		"file_revisions_enabled":            false,
		"file_revisions_keep_latest":        0,
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	effective = res.BodyFields["effective"].(map[string]any)
	assert.Equal(t, effective["query_max_result_rows"], any(float64(100)))
	assert.Equal(t, effective["file_revisions_enabled"], false)

	res = send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{
		"query_max_result_rows": 50,
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	res = send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+org.Slug+"/runtime-settings", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	effective = res.BodyFields["effective"].(map[string]any)
	assert.Equal(t, effective["query_max_result_rows"], any(float64(50)))

	res = send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug+"/runtime-settings", map[string]any{
		"query_max_result_rows": nil,
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	overrides := res.BodyFields["overrides"].(map[string]any)
	assert.Nil(t, overrides["query_max_result_rows"])
	effective = res.BodyFields["effective"].(map[string]any)
	assert.Equal(t, effective["query_max_result_rows"], any(float64(50)))
}

func TestOrganizationRuntimeSettingsCannotWeakenInstancePolicy(t *testing.T) {
	t.Parallel()
	app, org, _, token := setupWorkspaceOwner(t)

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug+"/runtime-settings", map[string]any{
		"query_max_result_rows":             database.DefaultQueryMaxResultRows + 1,
		"exports_background_max_bytes":      0,
		"schema_snapshot_freshness_seconds": 1,
		"file_revisions_keep_latest":        database.DefaultFileRevisionsKeepLatest + 1,
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "query_max_result_rows")
	assertValidationField(t, res, "schema_snapshot_freshness_seconds")
	assertValidationField(t, res, "file_revisions_keep_latest")
}

func TestOrganizationRuntimeSettingsQueryHistoryFields(t *testing.T) {
	t.Parallel()
	app, org, _, token := setupWorkspaceOwner(t)

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug+"/runtime-settings", map[string]any{
		"query_history_mode":            "local",
		"query_history_retention_count": 10,
		"query_favorites_mode":          "local",
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	overrides := res.BodyFields["overrides"].(map[string]any)
	assert.Equal(t, overrides["query_history_mode"], "local")
	assert.Equal(t, overrides["query_history_retention_count"], any(float64(10)))
	assert.Equal(t, overrides["query_favorites_mode"], "local")
	effective := res.BodyFields["effective"].(map[string]any)
	assert.Equal(t, effective["query_history_mode"], "local")
	assert.Equal(t, effective["query_history_retention_count"], any(float64(10)))
	assert.Equal(t, effective["query_favorites_mode"], "local")
}

func TestOrganizationRuntimeSettingsQueryHistoryFieldsCannotWeakenInstancePolicy(t *testing.T) {
	t.Parallel()
	app, org, _, token := setupWorkspaceOwner(t)
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "off"
		settings.QueryFavoritesMode = "off"
	})

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug+"/runtime-settings", map[string]any{
		"query_history_mode":            "local",
		"query_history_retention_count": database.DefaultQueryHistoryRetentionCount + 1,
		"query_favorites_mode":          "local",
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "query_history_mode")
	assertValidationField(t, res, "query_history_retention_count")
	assertValidationField(t, res, "query_favorites_mode")
}

func TestOrganizationRuntimeSettingsQueryHistoryRowsRemainFlag(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "query-history-rows-remain"), "Rows Remain Owner", "Rows Remain Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary Workspace", "")
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "backend"
	})
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Rows Remain Conn", "open")

	_, err := app.db.InsertQueryHistoryEntry(context.Background(), database.QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    owner.ID,
		SQL:          "select 1",
		Status:       "ok",
	}, database.DefaultQueryHistoryRetentionCount)
	if err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug+"/runtime-settings", map[string]any{
		"query_history_mode": "local",
	}, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["query_history_rows_remain"], true)
}

func TestPurgeOrganizationQueryHistory(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "purge-query-history"), "Purge History Owner", "Purge History Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary Workspace", "")
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.QueryHistoryMode = "backend"
	})
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Purge History Conn", "open")

	_, err := app.db.InsertQueryHistoryEntry(context.Background(), database.QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    owner.ID,
		SQL:          "select 1",
		Status:       "ok",
	}, database.DefaultQueryHistoryRetentionCount)
	if err != nil {
		t.Fatal(err)
	}

	hasRows, err := app.db.QueryHistoryHasRowsForOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRows {
		t.Fatalf("expected rows to exist before purge")
	}

	res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/query-history", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	hasRows, err = app.db.QueryHistoryHasRowsForOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasRows {
		t.Fatalf("expected no rows after purge")
	}
}

func TestPurgeOrganizationQueryFavorites(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, token, org := seedOrgOwner(t, app, uniqueEmail(t, "purge-query-favorites"), "Purge Favorites Owner", "Purge Favorites Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Primary Workspace", "")

	_, err := app.db.CreateQueryFavorite(context.Background(), database.QueryFavorite{
		WorkspaceID: ws.ID,
		AccountID:   owner.ID,
		Name:        "Top customers",
		SQL:         "select 1",
	})
	if err != nil {
		t.Fatal(err)
	}

	hasRows, err := app.db.QueryFavoritesHasRowsForOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRows {
		t.Fatalf("expected rows to exist before purge")
	}

	res := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+org.Slug+"/query-favorites", nil, token), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusNoContent)

	hasRows, err = app.db.QueryFavoritesHasRowsForOrg(context.Background(), org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasRows {
		t.Fatalf("expected no rows after purge")
	}
}
