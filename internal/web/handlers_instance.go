package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/settings"
)

// setup handles POST /api/setup. It works only while the instance has no
// instance admin; afterwards it returns 409.
func (app *application) setup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name             string `json:"name"`
		Email            string `json:"email"`
		Password         string `json:"password"`
		OrganizationName string `json:"organization_name"`
		OrganizationSlug string `json:"organization_slug"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	result, err := app.services.Identity.Setup(r.Context(), identity.SetupInput{
		Name: input.Name, Email: input.Email, Password: input.Password,
		OrganizationName: input.OrganizationName, OrganizationSlug: input.OrganizationSlug,
	}, app.requestAttributes(r))
	if err != nil {
		app.identityError(w, r, err)
		return
	}

	app.logInfo(r, "instance setup completed", slog.Int64("account_id", result.Account.ID), slog.Int64("org_id", result.Org.ID), slog.String("org_slug", result.Org.Slug), slog.String("auth_method", result.Method))
	err = response.JSON(w, http.StatusCreated, map[string]any{
		"account":      result.Account,
		"access_token": result.AccessToken,
		"organization": result.Org,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

// setupStatus reports whether the instance has already been bootstrapped.
func (app *application) setupStatus(w http.ResponseWriter, r *http.Request) {
	state, err := app.services.Identity.SetupStatus(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"configured":           state.Configured,
		"setup_requires_input": state.RequiresInput,
		"invitations_enabled":  app.invitationPolicy.Enabled(),
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

// authorizeInstanceAdmin writes the error response and returns false unless
// the caller is an instance admin. It serves instance routes whose work has
// no service of its own to authorize it.
func (app *application) authorizeInstanceAdmin(w http.ResponseWriter, r *http.Request) bool {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return false
	}
	if err := app.services.Identity.RequireInstanceAdmin(r.Context(), principal); err != nil {
		app.identityError(w, r, err)
		return false
	}
	return true
}

func toIdentityListQuery(q listQuery) identity.ListQuery {
	return identity.ListQuery{Search: q.Search, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize}
}

// listInstanceAdmins handles GET /api/v1/instance/admins.
func (app *application) listInstanceAdmins(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"account_id": "account_id",
		"created_at": "created_at",
	})
	if _, ok := r.URL.Query()["sort"]; !ok {
		q.Sort = "created_at"
	}
	if _, ok := r.URL.Query()["order"]; !ok {
		q.Order = "asc"
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	admins, err := app.services.Identity.ListInstanceAdmins(r.Context(), principal, toIdentityListQuery(q))
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, admins)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrganizations(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"slug":       "slug",
		"created_at": "created_at",
	})
	slug := r.URL.Query().Get("slug")
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	orgs, err := app.services.Identity.ListOrganizations(r.Context(), principal, toIdentityListQuery(q), slug)
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, orgs)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listInstanceAccounts(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"id":         "id",
		"email":      "email",
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	accounts, err := app.services.Identity.ListInstanceAccounts(r.Context(), principal, toIdentityListQuery(q))
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, accounts)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createInstanceAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	account, err := app.services.Identity.CreateInstanceAccount(r.Context(), principal, identity.CreateAccountInput{
		Name: input.Name, Email: input.Email, Password: input.Password,
	})
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "instance account created", slog.Int64("account_id", account.ID))
	err = response.JSON(w, http.StatusCreated, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getInstanceSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	current, err := app.services.Settings.GetInstance(r.Context(), principal)
	if err != nil {
		app.settingsError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, instanceSettingsResponse(current))
	if err != nil {
		app.serverError(w, r, err)
	}
}

// updateInstanceSettingsRequest mirrors settings.UpdateInstanceInput field for
// field so it converts directly; only the JSON names live here.
type updateInstanceSettingsRequest struct {
	InstanceName                  *string                `json:"instance_name"`
	InstanceDescription           *string                `json:"instance_description"`
	SupportEmail                  *string                `json:"support_email"`
	BaseURL                       *string                `json:"base_url"`
	JWTAccessTokenTTLSeconds      *int64                 `json:"jwt_access_token_ttl_seconds"`
	SessionsRevocationEnabled     *bool                  `json:"sessions_revocation_enabled"`
	QueryMaxResultRows            *int                   `json:"query_max_result_rows"`
	QueryCursorPageSize           *int                   `json:"query_cursor_page_size"`
	QueryMaxResultBytes           *int64                 `json:"query_max_result_bytes"`
	ExportsSyncMaxBytes           *int64                 `json:"exports_sync_max_bytes"`
	ExportsBackgroundMaxBytes     *int64                 `json:"exports_background_max_bytes"`
	FileRevisionsEnabled          *bool                  `json:"file_revisions_enabled"`
	FileRevisionsKeepLatest       *int                   `json:"file_revisions_keep_latest"`
	ErrorNotificationEmail        *string                `json:"error_notification_email"`
	LogLevel                      *string                `json:"log_level"`
	DatabaseQueryTracingEnabled   *bool                  `json:"database_query_tracing_enabled"`
	AccessLogsEnabled             *bool                  `json:"access_logs_enabled"`
	JobsWorkerCount               *int                   `json:"jobs_worker_count"`
	JobsPollIntervalSeconds       *int64                 `json:"jobs_poll_interval_seconds"`
	JobsClaimLeaseSeconds         *int64                 `json:"jobs_claim_lease_seconds"`
	JobsCompletedRetentionSeconds *int64                 `json:"jobs_completed_retention_seconds"`
	SMTPEnabled                   *bool                  `json:"smtp_enabled"`
	SMTPHost                      *string                `json:"smtp_host"`
	SMTPPort                      *int                   `json:"smtp_port"`
	SMTPUsername                  *string                `json:"smtp_username"`
	SMTPPassword                  settings.Patch[string] `json:"smtp_password"`
	SMTPFrom                      *string                `json:"smtp_from"`
	QueryHistoryMode              *string                `json:"query_history_mode"`
	QueryHistoryRetentionCount    *int                   `json:"query_history_retention_count"`
	QueryHistoryRetentionCountMax *int                   `json:"query_history_retention_count_max"`
	QueryFavoritesMode            *string                `json:"query_favorites_mode"`
	SQLiteLocalTargetsEnabled     *bool                  `json:"sqlite_local_targets_enabled"`
	SQLiteInMemoryTargetsEnabled  *bool                  `json:"sqlite_memory_targets_enabled"`
}

func (app *application) updateInstanceSettings(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input updateInstanceSettingsRequest
	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	updated, err := app.services.Settings.UpdateInstance(r.Context(), principal, settings.UpdateInstanceInput(input))
	if err != nil {
		app.settingsError(w, r, err)
		return
	}

	app.logInfo(r, "instance settings updated")
	err = response.JSON(w, http.StatusOK, instanceSettingsResponse(updated))
	if err != nil {
		app.serverError(w, r, err)
	}
}

func instanceSettingsResponse(in settings.Instance) map[string]any {
	return map[string]any{
		"instance_name":                     in.InstanceName,
		"instance_description":              in.InstanceDescription,
		"support_email":                     in.SupportEmail,
		"base_url":                          in.BaseURL,
		"jwt_access_token_ttl_seconds":      in.JWTAccessTokenTTLSeconds,
		"sessions_revocation_enabled":       in.SessionsRevocationEnabled,
		"query_max_result_rows":             in.QueryMaxResultRows,
		"query_cursor_page_size":            in.QueryCursorPageSize,
		"query_max_result_bytes":            in.QueryMaxResultBytes,
		"exports_sync_max_bytes":            in.ExportsSyncMaxBytes,
		"exports_background_max_bytes":      in.ExportsBackgroundMaxBytes,
		"file_revisions_enabled":            in.FileRevisionsEnabled,
		"file_revisions_keep_latest":        in.FileRevisionsKeepLatest,
		"error_notification_email":          in.ErrorNotificationEmail,
		"log_level":                         in.LogLevel,
		"database_query_tracing_enabled":    in.DatabaseQueryTracingEnabled,
		"access_logs_enabled":               in.AccessLogsEnabled,
		"jobs_worker_count":                 in.JobsWorkerCount,
		"jobs_poll_interval_seconds":        in.JobsPollIntervalSeconds,
		"jobs_claim_lease_seconds":          in.JobsClaimLeaseSeconds,
		"jobs_completed_retention_seconds":  in.JobsCompletedRetentionSeconds,
		"smtp_enabled":                      in.SMTPEnabled,
		"smtp_host":                         in.SMTPHost,
		"smtp_port":                         in.SMTPPort,
		"smtp_username":                     in.SMTPUsername,
		"smtp_password_configured":          in.SMTPPasswordEncrypted != "",
		"smtp_from":                         in.SMTPFrom,
		"query_history_mode":                in.QueryHistoryMode,
		"query_history_retention_count":     in.QueryHistoryRetentionCount,
		"query_history_retention_count_max": in.QueryHistoryRetentionCountMax,
		"query_favorites_mode":              in.QueryFavoritesMode,
		"sqlite_local_targets_enabled":      in.SQLiteLocalTargetsEnabled,
		"sqlite_memory_targets_enabled":     in.SQLiteInMemoryTargetsEnabled,
	}
}

// addInstanceAdmin handles POST /api/v1/instance/admins. It grants instance
// admin status to an existing account looked up by email.
func (app *application) addInstanceAdmin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	account, err := app.services.Identity.AddInstanceAdmin(r.Context(), principal, input.Email)
	if err != nil {
		app.identityError(w, r, err)
		return
	}

	app.logInfo(r, "instance admin added", slog.Int64("target_account_id", account.ID))
	w.WriteHeader(http.StatusNoContent)
}

// removeInstanceAdmin handles DELETE /api/v1/instance/admins/{account_id}. The
// last instance admin cannot be removed.
func (app *application) removeInstanceAdmin(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	err = app.services.Identity.RemoveInstanceAdmin(r.Context(), principal, accountID)
	if err != nil {
		app.identityError(w, r, err)
		return
	}

	app.logInfo(r, "instance admin removed", slog.Int64("target_account_id", accountID))
	w.WriteHeader(http.StatusNoContent)
}
