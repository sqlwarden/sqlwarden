package web

import (
	"errors"
	"net/http"

	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/settings"
)

// settingsError maps settings service failures to the standard error
// envelope.
func (app *application) settingsError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *settings.ValidationError
	if errors.As(err, &validation) {
		app.failedValidation(w, r, validation.Validator)
		return
	}
	app.serviceError(w, r, err)
}

func (app *application) getInstanceConfiguration(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeInstanceAdmin(w, r) {
		return
	}
	err := response.JSON(w, http.StatusOK, map[string]any{
		"deployment_managed":   true,
		"restart_required":     true,
		"http_port":            app.config.HTTPPort,
		"profile":              app.config.Profile,
		"log_format":           app.config.Log.Format,
		"database_driver":      app.config.DB.Driver,
		"database_automigrate": app.config.DB.Automigrate,
		"tls_enabled":          app.config.TLS.Enabled,
		"file_storage_mode":    app.config.Files.StorageMode,
		"file_storage_backend": app.config.Files.ActiveStorageBackend,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getOrganizationRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	view, err := app.services.Settings.GetOrg(r.Context(), principal, contextGetPathIDs(r).OrgID)
	if err != nil {
		app.settingsError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, organizationRuntimeSettingsResponse(view)); err != nil {
		app.serverError(w, r, err)
	}
}

func organizationRuntimeSettingsResponse(view settings.OrgView) map[string]any {
	overrides, effective, instance := view.Overrides, view.Effective, view.Instance
	return map[string]any{
		"overrides": map[string]any{
			"query_max_result_rows":         overrides.QueryMaxResultRows,
			"query_max_result_bytes":        overrides.QueryMaxResultBytes,
			"exports_sync_max_bytes":        overrides.ExportsSyncMaxBytes,
			"exports_background_max_bytes":  overrides.ExportsBackgroundMaxBytes,
			"file_revisions_enabled":        overrides.FileRevisionsEnabled,
			"file_revisions_keep_latest":    overrides.FileRevisionsKeepLatest,
			"query_history_mode":            overrides.QueryHistoryMode,
			"query_history_retention_count": overrides.QueryHistoryRetentionCount,
			"query_favorites_mode":          overrides.QueryFavoritesMode,
		},
		"effective": map[string]any{
			"query_max_result_rows":         effective.QueryMaxResultRows,
			"query_max_result_bytes":        effective.QueryMaxResultBytes,
			"exports_sync_max_bytes":        effective.ExportsSyncMaxBytes,
			"exports_background_max_bytes":  effective.ExportsBackgroundMaxBytes,
			"file_revisions_enabled":        effective.FileRevisionsEnabled,
			"file_revisions_keep_latest":    effective.FileRevisionsKeepLatest,
			"query_history_mode":            effective.QueryHistoryMode,
			"query_history_retention_count": effective.QueryHistoryRetentionCount,
			"query_favorites_mode":          effective.QueryFavoritesMode,
		},
		"constraints": map[string]any{
			"query_max_result_rows_max":         instance.QueryMaxResultRows,
			"query_max_result_bytes_max":        instance.QueryMaxResultBytes,
			"exports_sync_max_bytes_max":        instance.ExportsSyncMaxBytes,
			"exports_background_max_bytes_max":  instance.ExportsBackgroundMaxBytes,
			"file_revisions_available":          instance.FileRevisionsEnabled,
			"file_revisions_keep_latest_max":    instance.FileRevisionsKeepLatest,
			"query_history_retention_count_max": instance.QueryHistoryRetentionCountMax,
		},
	}
}

// updateOrgRuntimeSettingsRequest mirrors settings.UpdateOrgInput field for
// field so it converts directly; only the JSON names live here.
type updateOrgRuntimeSettingsRequest struct {
	QueryMaxResultRows         settings.Patch[int]    `json:"query_max_result_rows"`
	QueryMaxResultBytes        settings.Patch[int64]  `json:"query_max_result_bytes"`
	ExportsSyncMaxBytes        settings.Patch[int64]  `json:"exports_sync_max_bytes"`
	ExportsBackgroundMaxBytes  settings.Patch[int64]  `json:"exports_background_max_bytes"`
	FileRevisionsEnabled       settings.Patch[bool]   `json:"file_revisions_enabled"`
	FileRevisionsKeepLatest    settings.Patch[int]    `json:"file_revisions_keep_latest"`
	QueryHistoryMode           settings.Patch[string] `json:"query_history_mode"`
	QueryHistoryRetentionCount settings.Patch[int]    `json:"query_history_retention_count"`
	QueryFavoritesMode         settings.Patch[string] `json:"query_favorites_mode"`
}

func (app *application) updateOrganizationRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input updateOrgRuntimeSettingsRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	updated, err := app.services.Settings.UpdateOrg(r.Context(), principal, contextGetPathIDs(r).OrgID, settings.UpdateOrgInput(input))
	if err != nil {
		app.settingsError(w, r, err)
		return
	}
	resp := organizationRuntimeSettingsResponse(updated.OrgView)
	if updated.HistoryRowsRemain {
		resp["query_history_rows_remain"] = true
	}
	if updated.FavoritesRowsRemain {
		resp["query_favorites_rows_remain"] = true
	}
	if err := response.JSON(w, http.StatusOK, resp); err != nil {
		app.serverError(w, r, err)
	}
}

// purgeOrganizationQueryHistory deletes all backend-stored query history rows for
// an organization. Used after switching query_history_mode away from "backend" to
// clear rows that were retained under the previous mode.
func (app *application) purgeOrganizationQueryHistory(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	if err := app.services.Settings.PurgeOrgHistory(r.Context(), principal, contextGetPathIDs(r).OrgID); err != nil {
		app.settingsError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// purgeOrganizationQueryFavorites deletes all backend-stored query favorites for
// an organization. Used after switching query_favorites_mode away from "backend".
func (app *application) purgeOrganizationQueryFavorites(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	if err := app.services.Settings.PurgeOrgFavorites(r.Context(), principal, contextGetPathIDs(r).OrgID); err != nil {
		app.settingsError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
