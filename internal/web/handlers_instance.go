package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
	"github.com/uptrace/bun"
)

// setup handles POST /api/setup.
// Only callable when no instance admins exist (first-run). Creates an account and
// makes it the first instance admin. Returns 409 if already configured.
func (app *application) setup(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name             string              `json:"name"`
		Email            string              `json:"email"`
		Password         string              `json:"password"`
		OrganizationName string              `json:"organization_name"`
		OrganizationSlug string              `json:"organization_slug"`
		V                validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)
	input.OrganizationName = strings.TrimSpace(input.OrganizationName)
	input.OrganizationSlug = strings.TrimSpace(input.OrganizationSlug)

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.Email != "", "email", "Email is required.")
	input.V.CheckField(len(input.Password) >= 8, "password", "Password must be at least 8 characters.")

	organizationName := input.OrganizationName
	organizationSlug := input.OrganizationSlug
	if app.config.AccessMode != AccessModeSingleUser {
		input.V.CheckField(input.OrganizationName != "", "organization_name", "Organization name is required.")
		if organizationSlug == "" {
			organizationSlug = slugify(input.OrganizationName)
		}
		input.V.CheckField(organizationSlug != "", "organization_slug", "Organization slug is required.")
		if organizationSlug != "" {
			input.V.CheckField(isValidSlug(organizationSlug), "organization_slug", "Organization slug may only contain lowercase letters, numbers, and hyphens.")
			input.V.CheckField(len(organizationSlug) <= maxOrganizationSlugLength, "organization_slug", "Organization slug must be 64 characters or fewer.")
		}
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	configured, err := app.db.HasAnyInstanceAdmin(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if configured {
		app.errorMessage(w, r, http.StatusConflict, "Instance is already configured.", nil)
		return
	}

	if app.config.AccessMode != AccessModeSingleUser {
		_, found, err := app.db.GetOrgBySlug(r.Context(), organizationSlug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if found {
			input.V.AddFieldError("organization_slug", "An organization with this slug already exists.")
			app.failedValidation(w, r, input.V)
			return
		}
	} else {
		organizationName = singleUserDefaultOrgName
		organizationSlug = singleUserDefaultOrgSlug
		_, found, err := app.db.GetOrgBySlug(r.Context(), organizationSlug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if found {
			organizationSlug = singleUserDefaultOrgSlug + "-" + strings.ToLower(database.NewID())
		}
	}

	hashedPassword, err := password.Hash(input.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	account, org, authSession, err := app.createFirstRunSetup(r.Context(), input.Email, input.Name, &hashedPassword, organizationSlug, organizationName, r.Header.Get("User-Agent"), r.RemoteAddr)
	if err != nil {
		if isUniqueViolation(err) {
			input.V.AddFieldError("email", "An account or organization with these details already exists.")
			app.failedValidation(w, r, input.V)
			return
		}
		app.serverError(w, r, err)
		return
	}

	runtimeSettings, err := app.runtimeSettingsService().effectiveForOrg(r.Context(), nil)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	accessToken, _, err := token.IssueWithSessionTTL(strconv.FormatInt(account.ID, 10), authSession.ID, account.Email, account.Name, app.config.JWT.SecretKey, runtimeSettings.JWTAccessTokenTTL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	body := map[string]any{
		"account":      account,
		"access_token": accessToken,
		"organization": org,
	}

	app.logInfo(r, "instance setup completed", slog.Int64("account_id", account.ID), slog.Int64("org_id", org.ID), slog.String("org_slug", org.Slug), slog.String("access_mode", app.config.AccessMode))
	err = response.JSON(w, http.StatusCreated, body)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createFirstRunSetup(ctx context.Context, email, name string, hashedPassword *string, organizationSlug, organizationName, userAgent, remoteAddr string) (database.Account, database.Organization, database.AuthSession, error) {
	var account database.Account
	var org database.Organization
	var authSession database.AuthSession

	err := app.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		account, err = app.db.InsertAccountWithExecutor(ctx, tx, email, name, hashedPassword)
		if err != nil {
			return err
		}
		if err = app.db.InsertInstanceAdminWithExecutor(ctx, tx, account.ID); err != nil {
			return err
		}

		org, err = app.createOwnedOrganizationWithExecutor(ctx, tx, organizationSlug, organizationName, account.ID)
		if err != nil {
			return err
		}

		authSession, err = app.db.InsertAuthSessionWithExecutor(ctx, tx, account.ID, time.Now().Add(7*24*time.Hour), userAgent, remoteAddr)
		return err
	})
	if err != nil {
		return database.Account{}, database.Organization{}, database.AuthSession{}, err
	}

	app.enforcer.InvalidateOrgPolicy(org.ID)
	return account, org, authSession, nil
}

func (app *application) seedSingleUserOrganization(ctx context.Context, accountID int64) (database.Organization, error) {
	org, err := app.createOwnedOrganization(ctx, singleUserDefaultOrgSlug, singleUserDefaultOrgName, accountID)
	if err == nil {
		return org, nil
	}
	if !isUniqueViolation(err) {
		return database.Organization{}, err
	}

	// The first-run path normally owns an empty database, but avoid making
	// "local" a hard blocker if an operator pre-seeded data before setup.
	return app.createOwnedOrganization(ctx, fmt.Sprintf("%s-%d", singleUserDefaultOrgSlug, accountID), singleUserDefaultOrgName, accountID)
}

// setupStatus reports whether the instance has already been bootstrapped.
func (app *application) setupStatus(w http.ResponseWriter, r *http.Request) {
	configured, err := app.db.HasAnyInstanceAdmin(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"configured":      configured,
		"access_mode":     app.config.AccessMode,
		"deployment_mode": app.config.DeploymentMode,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
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

	admins, err := app.db.ListInstanceAdminsPage(r.Context(), database.ListInstanceAdminsParams{
		Search:   q.Search,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
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

	orgs, err := app.db.ListOrganizationsPage(r.Context(), database.ListOrganizationsParams{
		Search:   q.Search,
		Slug:     slug,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
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

	accounts, err := app.db.ListAccountsPage(r.Context(), database.ListAccountsParams{
		Search:   q.Search,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, accounts)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createInstanceAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string              `json:"name"`
		Email    string              `json:"email"`
		Password string              `json:"password"`
		V        validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(input.Email)

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.Email != "", "email", "Email is required.")
	input.V.CheckField(input.Email == "" || validator.IsEmail(input.Email), "email", "Enter a valid email address.")
	input.V.CheckField(len(input.Password) >= 8, "password", "Password must be at least 8 characters.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	_, exists, err := app.db.GetAccountByEmail(r.Context(), input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if exists {
		input.V.AddFieldError("email", "An account with this email already exists.")
		app.failedValidation(w, r, input.V)
		return
	}

	hashedPassword, err := password.Hash(input.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	account, err := app.db.InsertAccount(r.Context(), input.Email, input.Name, &hashedPassword)
	if err != nil {
		if isUniqueViolation(err) {
			input.V.AddFieldError("email", "An account with this email already exists.")
			app.failedValidation(w, r, input.V)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "instance account created", slog.Int64("account_id", account.ID))
	err = response.JSON(w, http.StatusCreated, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getInstanceSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := app.instanceSettings(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, app.instanceSettingsResponse(settings))
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateInstanceSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		InstanceName                   *string               `json:"instance_name"`
		InstanceDescription            *string               `json:"instance_description"`
		SupportEmail                   *string               `json:"support_email"`
		BaseURL                        *string               `json:"base_url"`
		PersonalSpacesEnabled          *bool                 `json:"personal_spaces_enabled"`
		JWTAccessTokenTTLSeconds       *int64                `json:"jwt_access_token_ttl_seconds"`
		SessionsRevocationEnabled      *bool                 `json:"sessions_revocation_enabled"`
		QueryMaxResultRows             *int                  `json:"query_max_result_rows"`
		QueryCursorPageSize            *int                  `json:"query_cursor_page_size"`
		QueryMaxResultBytes            *int64                `json:"query_max_result_bytes"`
		ExportsSyncMaxBytes            *int64                `json:"exports_sync_max_bytes"`
		ExportsBackgroundMaxBytes      *int64                `json:"exports_background_max_bytes"`
		SchemaSnapshotFreshnessSeconds *int64                `json:"schema_snapshot_freshness_seconds"`
		FileRevisionsEnabled           *bool                 `json:"file_revisions_enabled"`
		FileRevisionsKeepLatest        *int                  `json:"file_revisions_keep_latest"`
		ErrorNotificationEmail         *string               `json:"error_notification_email"`
		LogLevel                       *string               `json:"log_level"`
		DatabaseQueryTracingEnabled    *bool                 `json:"database_query_tracing_enabled"`
		AccessLogsEnabled              *bool                 `json:"access_logs_enabled"`
		JobsWorkerCount                *int                  `json:"jobs_worker_count"`
		JobsPollIntervalSeconds        *int64                `json:"jobs_poll_interval_seconds"`
		JobsClaimLeaseSeconds          *int64                `json:"jobs_claim_lease_seconds"`
		JobsCompletedRetentionSeconds  *int64                `json:"jobs_completed_retention_seconds"`
		SMTPEnabled                    *bool                 `json:"smtp_enabled"`
		SMTPHost                       *string               `json:"smtp_host"`
		SMTPPort                       *int                  `json:"smtp_port"`
		SMTPUsername                   *string               `json:"smtp_username"`
		SMTPPassword                   nullablePatch[string] `json:"smtp_password"`
		SMTPFrom                       *string               `json:"smtp_from"`
		QueryHistoryMode               *string               `json:"query_history_mode"`
		QueryHistoryRetentionCount     *int                  `json:"query_history_retention_count"`
		QueryHistoryRetentionCountMax  *int                  `json:"query_history_retention_count_max"`
		QueryFavoritesMode             *string               `json:"query_favorites_mode"`
		V                              validator.Validator   `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	hasPatch := input.InstanceName != nil ||
		input.InstanceDescription != nil ||
		input.SupportEmail != nil ||
		input.BaseURL != nil ||
		input.PersonalSpacesEnabled != nil ||
		input.JWTAccessTokenTTLSeconds != nil ||
		input.SessionsRevocationEnabled != nil ||
		input.QueryMaxResultRows != nil ||
		input.QueryCursorPageSize != nil ||
		input.QueryMaxResultBytes != nil ||
		input.ExportsSyncMaxBytes != nil ||
		input.ExportsBackgroundMaxBytes != nil ||
		input.SchemaSnapshotFreshnessSeconds != nil ||
		input.FileRevisionsEnabled != nil ||
		input.FileRevisionsKeepLatest != nil ||
		input.ErrorNotificationEmail != nil || input.LogLevel != nil ||
		input.DatabaseQueryTracingEnabled != nil || input.AccessLogsEnabled != nil || input.JobsWorkerCount != nil ||
		input.JobsPollIntervalSeconds != nil || input.JobsClaimLeaseSeconds != nil ||
		input.JobsCompletedRetentionSeconds != nil || input.SMTPEnabled != nil ||
		input.SMTPHost != nil || input.SMTPPort != nil || input.SMTPUsername != nil ||
		input.SMTPPassword.Set || input.SMTPFrom != nil ||
		input.QueryHistoryMode != nil || input.QueryHistoryRetentionCount != nil ||
		input.QueryHistoryRetentionCountMax != nil || input.QueryFavoritesMode != nil
	input.V.Check(hasPatch, "At least one setting is required.")
	if input.InstanceName != nil {
		*input.InstanceName = strings.TrimSpace(*input.InstanceName)
		input.V.CheckField(*input.InstanceName != "", "instance_name", "Instance name is required.")
		input.V.CheckField(validator.MaxRunes(*input.InstanceName, 120), "instance_name", "Instance name must be 120 characters or fewer.")
	}
	if input.InstanceDescription != nil {
		*input.InstanceDescription = strings.TrimSpace(*input.InstanceDescription)
		input.V.CheckField(validator.MaxRunes(*input.InstanceDescription, 500), "instance_description", "Description must be 500 characters or fewer.")
	}
	if input.SupportEmail != nil {
		*input.SupportEmail = strings.TrimSpace(*input.SupportEmail)
		input.V.CheckField(*input.SupportEmail == "" || validator.IsEmail(*input.SupportEmail), "support_email", "Support email must be a valid email address.")
	}
	if input.BaseURL != nil {
		*input.BaseURL = strings.TrimSpace(*input.BaseURL)
		input.V.CheckField(*input.BaseURL != "" && validator.IsURL(*input.BaseURL), "base_url", "Base URL must be a valid URL.")
	}
	if input.JWTAccessTokenTTLSeconds != nil {
		input.V.CheckField(*input.JWTAccessTokenTTLSeconds > 0 && *input.JWTAccessTokenTTLSeconds <= maxRuntimeDurationSeconds, "jwt_access_token_ttl_seconds", "Access token lifetime is outside the supported range.")
	}
	if input.QueryMaxResultRows != nil {
		input.V.CheckField(*input.QueryMaxResultRows > 0, "query_max_result_rows", "Query row limit must be greater than 0.")
	}
	if input.QueryCursorPageSize != nil {
		input.V.CheckField(*input.QueryCursorPageSize > 0, "query_cursor_page_size", "Query cursor page size must be greater than 0.")
	}
	if input.QueryMaxResultBytes != nil {
		input.V.CheckField(*input.QueryMaxResultBytes > 0, "query_max_result_bytes", "Query byte limit must be greater than 0.")
	}
	if input.ExportsSyncMaxBytes != nil {
		input.V.CheckField(*input.ExportsSyncMaxBytes > 0, "exports_sync_max_bytes", "Synchronous export limit must be greater than 0.")
	}
	if input.ExportsBackgroundMaxBytes != nil {
		input.V.CheckField(*input.ExportsBackgroundMaxBytes >= 0, "exports_background_max_bytes", "Background export limit must be 0 or greater.")
	}
	if input.SchemaSnapshotFreshnessSeconds != nil {
		input.V.CheckField(*input.SchemaSnapshotFreshnessSeconds > 0 && *input.SchemaSnapshotFreshnessSeconds <= maxRuntimeDurationSeconds, "schema_snapshot_freshness_seconds", "Schema snapshot freshness is outside the supported range.")
	}
	if input.FileRevisionsKeepLatest != nil {
		input.V.CheckField(*input.FileRevisionsKeepLatest >= 0, "file_revisions_keep_latest", "Revision retention must be 0 or greater.")
	}
	if input.ErrorNotificationEmail != nil {
		*input.ErrorNotificationEmail = strings.TrimSpace(*input.ErrorNotificationEmail)
		input.V.CheckField(*input.ErrorNotificationEmail == "" || validator.IsEmail(*input.ErrorNotificationEmail), "error_notification_email", "Error notification email must be a valid email address.")
	}
	if input.LogLevel != nil {
		*input.LogLevel = strings.ToLower(strings.TrimSpace(*input.LogLevel))
		input.V.CheckField(isSupportedLogLevel(*input.LogLevel), "log_level", "Log level must be debug, info, warn, or error.")
	}
	if input.JobsWorkerCount != nil {
		input.V.CheckField(*input.JobsWorkerCount > 0 && *input.JobsWorkerCount <= 256, "jobs_worker_count", "Worker count must be between 1 and 256.")
	}
	if input.JobsPollIntervalSeconds != nil {
		input.V.CheckField(*input.JobsPollIntervalSeconds > 0 && *input.JobsPollIntervalSeconds <= 3600, "jobs_poll_interval_seconds", "Poll interval must be between 1 and 3600 seconds.")
	}
	if input.JobsClaimLeaseSeconds != nil {
		input.V.CheckField(*input.JobsClaimLeaseSeconds > 0 && *input.JobsClaimLeaseSeconds <= 86400, "jobs_claim_lease_seconds", "Claim lease must be between 1 and 86400 seconds.")
	}
	if input.JobsCompletedRetentionSeconds != nil {
		input.V.CheckField(*input.JobsCompletedRetentionSeconds > 0 && *input.JobsCompletedRetentionSeconds <= 31536000, "jobs_completed_retention_seconds", "Completed job retention must be between 1 and 31536000 seconds.")
	}
	if input.SMTPHost != nil {
		*input.SMTPHost = strings.TrimSpace(*input.SMTPHost)
		input.V.CheckField(validator.MaxRunes(*input.SMTPHost, 253), "smtp_host", "SMTP host must be 253 characters or fewer.")
	}
	if input.SMTPPort != nil {
		input.V.CheckField(*input.SMTPPort > 0 && *input.SMTPPort <= 65535, "smtp_port", "SMTP port must be between 1 and 65535.")
	}
	if input.SMTPUsername != nil {
		*input.SMTPUsername = strings.TrimSpace(*input.SMTPUsername)
		input.V.CheckField(validator.MaxRunes(*input.SMTPUsername, 320), "smtp_username", "SMTP username must be 320 characters or fewer.")
	}
	if input.SMTPPassword.Value != nil {
		input.V.CheckField(validator.MaxRunes(*input.SMTPPassword.Value, 4096), "smtp_password", "SMTP password is too long.")
	}
	if input.SMTPFrom != nil {
		*input.SMTPFrom = strings.TrimSpace(*input.SMTPFrom)
		input.V.CheckField(validator.MaxRunes(*input.SMTPFrom, 500), "smtp_from", "SMTP sender must be 500 characters or fewer.")
	}
	if input.QueryHistoryMode != nil {
		input.V.CheckField(isSupportedQueryHistoryMode(*input.QueryHistoryMode), "query_history_mode", "Query history mode must be backend, local, or off.")
	}
	if input.QueryHistoryRetentionCount != nil {
		input.V.CheckField(*input.QueryHistoryRetentionCount >= 1, "query_history_retention_count", "Retention count must be at least 1.")
	}
	if input.QueryHistoryRetentionCountMax != nil {
		input.V.CheckField(*input.QueryHistoryRetentionCountMax >= 1, "query_history_retention_count_max", "Retention count maximum must be at least 1.")
	}
	if input.QueryFavoritesMode != nil {
		input.V.CheckField(isSupportedQueryHistoryMode(*input.QueryFavoritesMode), "query_favorites_mode", "Query favorites mode must be backend, local, or off.")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	currentSettings, err := app.instanceSettings(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	nextSettings := currentSettings
	if input.InstanceName != nil {
		nextSettings.InstanceName = *input.InstanceName
	}
	if input.InstanceDescription != nil {
		nextSettings.InstanceDescription = *input.InstanceDescription
	}
	if input.SupportEmail != nil {
		nextSettings.SupportEmail = *input.SupportEmail
	}
	if input.BaseURL != nil {
		nextSettings.BaseURL = *input.BaseURL
	}
	if input.PersonalSpacesEnabled != nil {
		nextSettings.PersonalSpacesEnabled = *input.PersonalSpacesEnabled
	}
	if input.JWTAccessTokenTTLSeconds != nil {
		nextSettings.JWTAccessTokenTTLSeconds = *input.JWTAccessTokenTTLSeconds
	}
	if input.SessionsRevocationEnabled != nil {
		nextSettings.SessionsRevocationEnabled = *input.SessionsRevocationEnabled
	}
	if input.QueryMaxResultRows != nil {
		nextSettings.QueryMaxResultRows = *input.QueryMaxResultRows
	}
	if input.QueryCursorPageSize != nil {
		nextSettings.QueryCursorPageSize = *input.QueryCursorPageSize
	}
	if input.QueryMaxResultBytes != nil {
		nextSettings.QueryMaxResultBytes = *input.QueryMaxResultBytes
	}
	if input.ExportsSyncMaxBytes != nil {
		nextSettings.ExportsSyncMaxBytes = *input.ExportsSyncMaxBytes
	}
	if input.ExportsBackgroundMaxBytes != nil {
		nextSettings.ExportsBackgroundMaxBytes = *input.ExportsBackgroundMaxBytes
	}
	if input.SchemaSnapshotFreshnessSeconds != nil {
		nextSettings.SchemaSnapshotFreshnessSeconds = *input.SchemaSnapshotFreshnessSeconds
	}
	if input.FileRevisionsEnabled != nil {
		nextSettings.FileRevisionsEnabled = *input.FileRevisionsEnabled
	}
	if input.FileRevisionsKeepLatest != nil {
		nextSettings.FileRevisionsKeepLatest = *input.FileRevisionsKeepLatest
	}
	if input.ErrorNotificationEmail != nil {
		nextSettings.ErrorNotificationEmail = *input.ErrorNotificationEmail
	}
	if input.LogLevel != nil {
		nextSettings.LogLevel = *input.LogLevel
	}
	if input.DatabaseQueryTracingEnabled != nil {
		nextSettings.DatabaseQueryTracingEnabled = *input.DatabaseQueryTracingEnabled
	}
	if input.AccessLogsEnabled != nil {
		nextSettings.AccessLogsEnabled = *input.AccessLogsEnabled
	}
	if input.JobsWorkerCount != nil {
		nextSettings.JobsWorkerCount = *input.JobsWorkerCount
	}
	if input.JobsPollIntervalSeconds != nil {
		nextSettings.JobsPollIntervalSeconds = *input.JobsPollIntervalSeconds
	}
	if input.JobsClaimLeaseSeconds != nil {
		nextSettings.JobsClaimLeaseSeconds = *input.JobsClaimLeaseSeconds
	}
	if input.JobsCompletedRetentionSeconds != nil {
		nextSettings.JobsCompletedRetentionSeconds = *input.JobsCompletedRetentionSeconds
	}
	if input.SMTPEnabled != nil {
		nextSettings.SMTPEnabled = *input.SMTPEnabled
	}
	if input.SMTPHost != nil {
		nextSettings.SMTPHost = *input.SMTPHost
	}
	if input.SMTPPort != nil {
		nextSettings.SMTPPort = *input.SMTPPort
	}
	if input.SMTPUsername != nil {
		nextSettings.SMTPUsername = *input.SMTPUsername
	}
	if input.SMTPFrom != nil {
		nextSettings.SMTPFrom = *input.SMTPFrom
	}
	if input.SMTPPassword.Set {
		nextSettings.SMTPPasswordEncrypted = ""
		if input.SMTPPassword.Value != nil && *input.SMTPPassword.Value != "" {
			nextSettings.SMTPPasswordEncrypted, err = app.keyring.Encrypt(*input.SMTPPassword.Value)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
		}
	}
	if input.QueryHistoryMode != nil {
		nextSettings.QueryHistoryMode = *input.QueryHistoryMode
	}
	if input.QueryHistoryRetentionCount != nil {
		nextSettings.QueryHistoryRetentionCount = *input.QueryHistoryRetentionCount
	}
	if input.QueryHistoryRetentionCountMax != nil {
		nextSettings.QueryHistoryRetentionCountMax = *input.QueryHistoryRetentionCountMax
	}
	if input.QueryFavoritesMode != nil {
		nextSettings.QueryFavoritesMode = *input.QueryFavoritesMode
	}
	if err := validateInstanceSettings(nextSettings); err != nil {
		input.V.AddError(err.Error())
	}
	if app.config.Files.StorageMode == FilesStorageModeFile && nextSettings.FileRevisionsEnabled {
		input.V.AddFieldError("file_revisions_enabled", "File revisions are not supported with file storage mode.")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	settings, err := app.db.UpsertInstanceSettings(r.Context(), nextSettings)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.queueRuntimeOperations(settings)

	if currentSettings.PersonalSpacesEnabled && !settings.PersonalSpacesEnabled {
		if err := app.dropPersonalSpaceSessions(r.Context()); err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	app.logInfo(r, "instance settings updated", slog.Bool("personal_spaces_enabled", settings.PersonalSpacesEnabled))
	err = response.JSON(w, http.StatusOK, app.instanceSettingsResponse(settings))
	if err != nil {
		app.serverError(w, r, err)
	}
}

// addInstanceAdmin handles POST /api/v1/instance/admins.
// Grants instance admin status to an existing account (looked up by email).
func (app *application) addInstanceAdmin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string              `json:"email"`
		V     validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Email != "", "email", "Email is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account, found, err := app.db.GetAccountByEmail(r.Context(), input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.InsertInstanceAdmin(r.Context(), account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "instance admin added", slog.Int64("target_account_id", account.ID))
	w.WriteHeader(http.StatusNoContent)
}

// removeInstanceAdmin handles DELETE /api/v1/instance/admins/{account_id}.
// Cannot remove the last instance admin.
func (app *application) removeInstanceAdmin(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "account_id")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	err = app.db.RemoveInstanceAdmin(r.Context(), accountID)
	if errors.Is(err, database.ErrLastInstanceAdmin) {
		v := validator.Validator{}
		v.AddError("Cannot remove the last instance admin.")
		app.failedValidation(w, r, v)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "instance admin removed", slog.Int64("target_account_id", accountID))
	w.WriteHeader(http.StatusNoContent)
}
