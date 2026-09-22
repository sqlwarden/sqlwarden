package web

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	settingsapp "github.com/sqlwarden/internal/settings"
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

func (app *application) getCapabilities(w http.ResponseWriter, r *http.Request) {
	if err := response.JSON(w, http.StatusOK, map[string]any{
		"edition":      app.edition.Name(),
		"capabilities": app.edition.Entitlements().Clone(),
	}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getInstanceConfiguration(w http.ResponseWriter, r *http.Request) {
	err := response.JSON(w, http.StatusOK, map[string]any{
		"deployment_managed":   true,
		"restart_required":     true,
		"http_port":            app.config.HTTPPort,
		"deployment_mode":      app.config.DeploymentMode,
		"access_mode":          app.config.AccessMode,
		"log_format":           app.config.Log.Format,
		"database_driver":      app.config.DB.Driver,
		"database_automigrate": app.config.DB.Automigrate,
		"tls_enabled":          app.config.TLS.Enabled,
		"file_storage_mode":    app.config.Files.StorageMode,
		"file_storage_backend": app.config.Files.ActiveStorageBackend,
		"edition":              app.edition.Name(),
		"capabilities":         app.edition.Entitlements().Clone(),
	})
	if err != nil {
		app.serverError(w, r, err)
	}
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
	effective, err := app.settingsService().EffectiveForOrg(r.Context(), &org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, organizationRuntimeSettingsResponse(overrides, effective, instance)); err != nil {
		app.serverError(w, r, err)
	}
}

func organizationRuntimeSettingsResponse(overrides database.OrganizationRuntimeSettings, effective settingsapp.Effective, instance database.InstanceSettings) map[string]any {
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
	for _, violation := range settingsapp.ValidateOrganizationOverrides(settings, instance) {
		input.V.CheckField(false, violation.Field, violation.Message)
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}
	previousEffective, err := app.settingsService().EffectiveForOrg(r.Context(), &org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	settings, err = app.db.UpsertOrganizationRuntimeSettings(r.Context(), settings)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	effective, err := app.settingsService().EffectiveForOrg(r.Context(), &org.ID)
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
