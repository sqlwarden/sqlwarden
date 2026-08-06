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
	assert.Equal(t, res.BodyFields["error_notification_email"], "errors@example.com")

	settings, err := app.runtimeSettingsService().effectiveForOrg(context.Background(), nil)
	assert.Nil(t, err)
	assert.Equal(t, settings.QueryMaxResultRows, 2000)
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

func TestInstanceRuntimeSettingsValidationIsAtomic(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "runtime-validation@example.com", "Runtime Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/instance/settings", map[string]any{
		"query_max_result_rows":             0,
		"schema_snapshot_freshness_seconds": -1,
		"error_notification_email":          "invalid",
	}, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnprocessableEntity)
	assertValidationField(t, res, "query_max_result_rows")
	assertValidationField(t, res, "schema_snapshot_freshness_seconds")
	assertValidationField(t, res, "error_notification_email")

	settings, err := app.instanceSettings(context.Background())
	assert.Nil(t, err)
	assert.Equal(t, settings.QueryMaxResultRows, database.DefaultQueryMaxResultRows)
}

func TestInstanceBootstrapConfigurationIsSanitized(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	adminToken := setupInstance(t, app, "configuration-admin@example.com", "Configuration Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/instance/configuration", nil, adminToken), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["deployment_managed"], true)
	assert.Equal(t, res.BodyFields["restart_required"], true)
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
