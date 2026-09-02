package web

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

type nullablePatch[T any] struct {
	Set   bool
	Value *T
}

func (p *nullablePatch[T]) UnmarshalJSON(data []byte) error {
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

func (app *application) getInstanceConfiguration(w http.ResponseWriter, r *http.Request) {
	err := response.JSON(w, http.StatusOK, map[string]any{
		"deployment_managed":   true,
		"restart_required":     true,
		"http_port":            app.config.HTTPPort,
		"mode":                 app.config.Mode,
		"capabilities":         app.config.productCapabilities(),
		"log_format":           app.config.Log.Format,
		"database_driver":      app.config.DB.Driver,
		"database_automigrate": app.config.DB.Automigrate,
		"tls_enabled":          app.config.TLS.Enabled,
		"file_storage_mode":    app.config.Files.StorageMode,
		"file_storage_backend": app.config.Files.ActiveStorageBackend,
		"sqlite_local_enabled": containsString(app.config.Drivers.SQLite.AllowedSources, SQLiteDriverSourceLocal),
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (app *application) getOrganizationRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	overrides, _, err := app.db.GetOrganizationRuntimeSettings(r.Context(), org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	instance, err := app.instanceSettings(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	effective, err := app.runtimeSettingsService().effectiveForOrg(r.Context(), &org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, organizationRuntimeSettingsResponse(overrides, effective, instance)); err != nil {
		app.serverError(w, r, err)
	}
}

func organizationRuntimeSettingsResponse(overrides database.OrganizationRuntimeSettings, effective effectiveRuntimeSettings, instance database.InstanceSettings) map[string]any {
	return map[string]any{
		"overrides": map[string]any{
			"query_max_result_rows":             overrides.QueryMaxResultRows,
			"query_max_result_bytes":            overrides.QueryMaxResultBytes,
			"exports_sync_max_bytes":            overrides.ExportsSyncMaxBytes,
			"exports_background_max_bytes":      overrides.ExportsBackgroundMaxBytes,
			"schema_snapshot_freshness_seconds": overrides.SchemaSnapshotFreshnessSeconds,
			"file_revisions_enabled":            overrides.FileRevisionsEnabled,
			"file_revisions_keep_latest":        overrides.FileRevisionsKeepLatest,
			"query_history_mode":                overrides.QueryHistoryMode,
			"query_history_retention_count":     overrides.QueryHistoryRetentionCount,
			"query_favorites_mode":              overrides.QueryFavoritesMode,
		},
		"effective": map[string]any{
			"query_max_result_rows":             effective.QueryMaxResultRows,
			"query_max_result_bytes":            effective.QueryMaxResultBytes,
			"exports_sync_max_bytes":            effective.ExportsSyncMaxBytes,
			"exports_background_max_bytes":      effective.ExportsBackgroundMaxBytes,
			"schema_snapshot_freshness_seconds": int64(effective.SchemaSnapshotFreshness.Seconds()),
			"file_revisions_enabled":            effective.FileRevisionsEnabled,
			"file_revisions_keep_latest":        effective.FileRevisionsKeepLatest,
			"query_history_mode":                effective.QueryHistoryMode,
			"query_history_retention_count":     effective.QueryHistoryRetentionCount,
			"query_favorites_mode":              effective.QueryFavoritesMode,
		},
		"constraints": map[string]any{
			"query_max_result_rows_max":             instance.QueryMaxResultRows,
			"query_max_result_bytes_max":            instance.QueryMaxResultBytes,
			"exports_sync_max_bytes_max":            instance.ExportsSyncMaxBytes,
			"exports_background_max_bytes_max":      instance.ExportsBackgroundMaxBytes,
			"schema_snapshot_freshness_seconds_min": instance.SchemaSnapshotFreshnessSeconds,
			"file_revisions_available":              instance.FileRevisionsEnabled,
			"file_revisions_keep_latest_max":        instance.FileRevisionsKeepLatest,
			"query_history_retention_count_max":     instance.QueryHistoryRetentionCountMax,
		},
	}
}

func (app *application) updateOrganizationRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	var input struct {
		QueryMaxResultRows             nullablePatch[int]    `json:"query_max_result_rows"`
		QueryMaxResultBytes            nullablePatch[int64]  `json:"query_max_result_bytes"`
		ExportsSyncMaxBytes            nullablePatch[int64]  `json:"exports_sync_max_bytes"`
		ExportsBackgroundMaxBytes      nullablePatch[int64]  `json:"exports_background_max_bytes"`
		SchemaSnapshotFreshnessSeconds nullablePatch[int64]  `json:"schema_snapshot_freshness_seconds"`
		FileRevisionsEnabled           nullablePatch[bool]   `json:"file_revisions_enabled"`
		FileRevisionsKeepLatest        nullablePatch[int]    `json:"file_revisions_keep_latest"`
		QueryHistoryMode               nullablePatch[string] `json:"query_history_mode"`
		QueryHistoryRetentionCount     nullablePatch[int]    `json:"query_history_retention_count"`
		QueryFavoritesMode             nullablePatch[string] `json:"query_favorites_mode"`
		V                              validator.Validator   `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	hasPatch := input.QueryMaxResultRows.Set || input.QueryMaxResultBytes.Set ||
		input.ExportsSyncMaxBytes.Set || input.ExportsBackgroundMaxBytes.Set ||
		input.SchemaSnapshotFreshnessSeconds.Set || input.FileRevisionsEnabled.Set ||
		input.FileRevisionsKeepLatest.Set || input.QueryHistoryMode.Set ||
		input.QueryHistoryRetentionCount.Set || input.QueryFavoritesMode.Set
	input.V.Check(hasPatch, "At least one setting is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	settings, _, err := app.db.GetOrganizationRuntimeSettings(r.Context(), org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	settings.OrgID = org.ID
	if input.QueryMaxResultRows.Set {
		settings.QueryMaxResultRows = input.QueryMaxResultRows.Value
	}
	if input.QueryMaxResultBytes.Set {
		settings.QueryMaxResultBytes = input.QueryMaxResultBytes.Value
	}
	if input.ExportsSyncMaxBytes.Set {
		settings.ExportsSyncMaxBytes = input.ExportsSyncMaxBytes.Value
	}
	if input.ExportsBackgroundMaxBytes.Set {
		settings.ExportsBackgroundMaxBytes = input.ExportsBackgroundMaxBytes.Value
	}
	if input.SchemaSnapshotFreshnessSeconds.Set {
		settings.SchemaSnapshotFreshnessSeconds = input.SchemaSnapshotFreshnessSeconds.Value
	}
	if input.FileRevisionsEnabled.Set {
		settings.FileRevisionsEnabled = input.FileRevisionsEnabled.Value
	}
	if input.FileRevisionsKeepLatest.Set {
		settings.FileRevisionsKeepLatest = input.FileRevisionsKeepLatest.Value
	}
	if input.QueryHistoryMode.Set {
		settings.QueryHistoryMode = input.QueryHistoryMode.Value
	}
	if input.QueryHistoryRetentionCount.Set {
		settings.QueryHistoryRetentionCount = input.QueryHistoryRetentionCount.Value
	}
	if input.QueryFavoritesMode.Set {
		settings.QueryFavoritesMode = input.QueryFavoritesMode.Value
	}

	instance, err := app.instanceSettings(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	validateOrganizationRuntimeSettings(&input.V, settings, instance)
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}
	previousEffective, err := app.runtimeSettingsService().effectiveForOrg(r.Context(), &org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	settings, err = app.db.UpsertOrganizationRuntimeSettings(r.Context(), settings)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	effective, err := app.runtimeSettingsService().effectiveForOrg(r.Context(), &org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	resp := organizationRuntimeSettingsResponse(settings, effective, instance)
	if previousEffective.QueryHistoryMode == "backend" && effective.QueryHistoryMode != "backend" {
		hasRows, err := app.db.QueryHistoryHasRowsForOrg(r.Context(), org.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if hasRows {
			resp["query_history_rows_remain"] = true
		}
	}
	if previousEffective.QueryFavoritesMode == "backend" && effective.QueryFavoritesMode != "backend" {
		hasRows, err := app.db.QueryFavoritesHasRowsForOrg(r.Context(), org.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if hasRows {
			resp["query_favorites_rows_remain"] = true
		}
	}
	if err := response.JSON(w, http.StatusOK, resp); err != nil {
		app.serverError(w, r, err)
	}
}

func validateOrganizationRuntimeSettings(v *validator.Validator, settings database.OrganizationRuntimeSettings, instance database.InstanceSettings) {
	if settings.QueryMaxResultRows != nil {
		v.CheckField(*settings.QueryMaxResultRows > 0 && *settings.QueryMaxResultRows <= instance.QueryMaxResultRows,
			"query_max_result_rows", "Query row limit must be greater than 0 and no greater than the instance limit.")
	}
	if settings.QueryMaxResultBytes != nil {
		v.CheckField(*settings.QueryMaxResultBytes > 0 && *settings.QueryMaxResultBytes <= instance.QueryMaxResultBytes,
			"query_max_result_bytes", "Query byte limit must be greater than 0 and no greater than the instance limit.")
	}
	if settings.ExportsSyncMaxBytes != nil {
		v.CheckField(*settings.ExportsSyncMaxBytes > 0 && *settings.ExportsSyncMaxBytes <= instance.ExportsSyncMaxBytes,
			"exports_sync_max_bytes", "Synchronous export limit must be greater than 0 and no greater than the instance limit.")
	}
	if settings.ExportsBackgroundMaxBytes != nil {
		valid := *settings.ExportsBackgroundMaxBytes >= 0
		if instance.ExportsBackgroundMaxBytes > 0 {
			valid = valid && *settings.ExportsBackgroundMaxBytes > 0 && *settings.ExportsBackgroundMaxBytes <= instance.ExportsBackgroundMaxBytes
		}
		v.CheckField(valid, "exports_background_max_bytes", "Background export limit must not exceed the instance limit; 0 is allowed only when the instance is unlimited.")
	}
	if settings.SchemaSnapshotFreshnessSeconds != nil {
		v.CheckField(*settings.SchemaSnapshotFreshnessSeconds >= instance.SchemaSnapshotFreshnessSeconds && *settings.SchemaSnapshotFreshnessSeconds <= maxRuntimeDurationSeconds,
			"schema_snapshot_freshness_seconds", "Schema snapshot freshness must be at least the instance interval.")
	}
	if settings.FileRevisionsEnabled != nil {
		v.CheckField(!*settings.FileRevisionsEnabled || instance.FileRevisionsEnabled,
			"file_revisions_enabled", "File revisions cannot be enabled when disabled for the instance.")
	}
	if settings.FileRevisionsKeepLatest != nil {
		v.CheckField(*settings.FileRevisionsKeepLatest >= 0 && *settings.FileRevisionsKeepLatest <= instance.FileRevisionsKeepLatest,
			"file_revisions_keep_latest", "Revision retention must be 0 or greater and no greater than the instance limit.")
	}
	if settings.QueryHistoryMode != nil {
		v.CheckField(isSupportedQueryHistoryMode(*settings.QueryHistoryMode),
			"query_history_mode", "Query history mode must be backend, local, or off.")
		v.CheckField(instance.QueryHistoryMode != "off" || *settings.QueryHistoryMode == "off",
			"query_history_mode", "Query history cannot be enabled when disabled for the instance.")
	}
	if settings.QueryHistoryRetentionCount != nil {
		v.CheckField(*settings.QueryHistoryRetentionCount >= 1 && *settings.QueryHistoryRetentionCount <= instance.QueryHistoryRetentionCount,
			"query_history_retention_count", "Retention count must be at least 1 and no greater than the instance limit.")
	}
	if settings.QueryFavoritesMode != nil {
		v.CheckField(isSupportedQueryHistoryMode(*settings.QueryFavoritesMode),
			"query_favorites_mode", "Query favorites mode must be backend, local, or off.")
		v.CheckField(instance.QueryFavoritesMode != "off" || *settings.QueryFavoritesMode == "off",
			"query_favorites_mode", "Query favorites cannot be enabled when disabled for the instance.")
	}
}

// purgeOrganizationQueryHistory deletes all backend-stored query history rows for
// an organization. Used after switching query_history_mode away from "backend" to
// clear rows that were retained under the previous mode.
func (app *application) purgeOrganizationQueryHistory(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

	if err := app.db.ClearQueryHistoryForOrg(r.Context(), org.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// purgeOrganizationQueryFavorites deletes all backend-stored query favorites for
// an organization. Used after switching query_favorites_mode away from "backend".
func (app *application) purgeOrganizationQueryFavorites(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

	if err := app.db.ClearQueryFavoritesForOrg(r.Context(), org.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
