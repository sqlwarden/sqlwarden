package main

import (
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

// --- Pagination helpers ---

func parsePagination(r *http.Request) (page, limit int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	return
}

type paginatedResult struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
	Page  int         `json:"page"`
	Limit int         `json:"limit"`
}

// --- Admin: orgs ---

func (app *application) adminListOrgs(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	tenants, total, err := app.db.ListAllTenants(page, limit)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, paginatedResult{Data: tenants, Total: total, Page: page, Limit: limit}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createOrg(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Slug       string `json:"slug"`
		Name       string `json:"name"`
		OwnerEmail string `json:"owner_email"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	if !rgxSlugValid.MatchString(input.Slug) {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "slug must be lowercase alphanumeric with hyphens", nil)
		return
	}

	// Owner must have an existing account
	owner, ok, err := app.db.GetAccountByEmail(input.OwnerEmail)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !ok {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "account not found for owner_email", nil)
		return
	}

	// Slug must be unique
	_, exists, err := app.db.GetTenantBySlug(input.Slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if exists {
		app.errorMessage(w, r, http.StatusConflict, "slug already taken", nil)
		return
	}

	tenant, err := app.db.InsertTenant(input.Slug, input.Name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := app.db.AddTenantMember(tenant.ID, owner.ID, "owner"); err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := app.enforcer.SeedOrgPolicies(input.Slug, owner.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusCreated, tenant); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin: accounts ---

func (app *application) adminListAccounts(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePagination(r)
	accounts, total, err := app.db.ListAllAccounts(page, limit)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, paginatedResult{Data: accounts, Total: total, Page: page, Limit: limit}); err != nil {
		app.serverError(w, r, err)
	}
}

// --- Admin: settings ---

func (app *application) getInstanceSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := app.db.GetAllSettings()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, settings); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateInstanceSetting(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	allowed := map[string]bool{
		"auth_method":           true,
		"personal_orgs_enabled": true,
		"sso_enforced":          true,
	}
	if !allowed[input.Key] {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "unknown setting key", nil)
		return
	}

	if err := app.db.UpdateSetting(input.Key, input.Value); err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
