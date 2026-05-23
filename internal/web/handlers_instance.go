package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// setup handles POST /api/setup.
// Only callable when no instance admins exist (first-run). Creates an account and
// makes it the first instance admin. Returns 409 if already configured.
func (app *application) setup(w http.ResponseWriter, r *http.Request) {
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

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.Email != "", "email", "Email is required.")
	input.V.CheckField(len(input.Password) >= 8, "password", "Password must be at least 8 characters.")
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

	err = app.db.InsertInstanceAdmin(r.Context(), account.ID)
	if err != nil {
		if isUniqueViolation(err) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		app.serverError(w, r, err)
		return
	}

	var org *database.Organization
	if app.config.AccessMode == AccessModeSingleUser {
		seededOrg, err := app.seedSingleUserOrganization(r.Context(), account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		org = &seededOrg
	}

	accessToken, _, err := app.newAuthenticationToken(account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	body := map[string]any{
		"account":      account,
		"access_token": accessToken,
	}
	if org != nil {
		body["organization"] = org
	}

	err = response.JSON(w, http.StatusCreated, body)
	if err != nil {
		app.serverError(w, r, err)
	}
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
		"configured": configured,
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
	enabled, err := app.personalSpacesEnabled(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"personal_spaces_enabled": enabled,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateInstanceSettings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PersonalSpacesEnabled *bool               `json:"personal_spaces_enabled"`
		V                     validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.PersonalSpacesEnabled != nil, "personal_spaces_enabled", "Personal spaces setting is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	currentEnabled, err := app.personalSpacesEnabled(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	settings, err := app.db.UpsertInstanceSettings(r.Context(), *input.PersonalSpacesEnabled)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if currentEnabled && !settings.PersonalSpacesEnabled {
		if err := app.dropPersonalSpaceSessions(r.Context()); err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"personal_spaces_enabled": settings.PersonalSpacesEnabled,
	})
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

	n, err := app.db.CountInstanceAdmins(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if n <= 1 {
		v := validator.Validator{}
		v.AddError("Cannot remove the last instance admin.")
		app.failedValidation(w, r, v)
		return
	}

	err = app.db.RemoveInstanceAdmin(r.Context(), accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
