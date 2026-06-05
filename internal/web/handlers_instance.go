package web

import (
	"context"
	"errors"
	"fmt"
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

	accessToken, _, err := token.IssueWithSessionTTL(strconv.FormatInt(account.ID, 10), authSession.ID, account.Email, account.Name, app.config.JWT.SecretKey, app.config.JWT.AccessTokenTTL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	body := map[string]any{
		"account":      account,
		"access_token": accessToken,
		"organization": org,
	}

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
		"configured":  configured,
		"access_mode": app.config.AccessMode,
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
		InstanceName          *string             `json:"instance_name"`
		InstanceDescription   *string             `json:"instance_description"`
		SupportEmail          *string             `json:"support_email"`
		PublicURL             *string             `json:"public_url"`
		PersonalSpacesEnabled *bool               `json:"personal_spaces_enabled"`
		V                     validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	hasPatch := input.InstanceName != nil ||
		input.InstanceDescription != nil ||
		input.SupportEmail != nil ||
		input.PublicURL != nil ||
		input.PersonalSpacesEnabled != nil
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
	if input.PublicURL != nil {
		*input.PublicURL = strings.TrimSpace(*input.PublicURL)
		input.V.CheckField(*input.PublicURL == "" || validator.IsURL(*input.PublicURL), "public_url", "Public URL must be a valid URL.")
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
	if input.PublicURL != nil {
		nextSettings.PublicURL = *input.PublicURL
	}
	if input.PersonalSpacesEnabled != nil {
		nextSettings.PersonalSpacesEnabled = *input.PersonalSpacesEnabled
	}

	settings, err := app.db.UpsertInstanceSettings(r.Context(), nextSettings)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if currentSettings.PersonalSpacesEnabled && !settings.PersonalSpacesEnabled {
		if err := app.dropPersonalSpaceSessions(r.Context()); err != nil {
			app.serverError(w, r, err)
			return
		}
	}

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

	w.WriteHeader(http.StatusNoContent)
}
