package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listRoles(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	roles, err := app.db.GetWorkspaceRolesByTenant(tenant.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if roles == nil {
		roles = []database.WorkspaceRole{}
	}

	err = response.JSON(w, http.StatusOK, roles)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createRole(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.Name), "name", "Name is required")
	v.CheckField(validator.MaxRunes(input.Name, 100), "name", "Must not be more than 100 characters")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	role, err := app.db.InsertWorkspaceRole(tenant.ID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, role)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getRole(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)
	roleID := chi.URLParam(r, "role_id")

	role, found, err := app.db.GetWorkspaceRole(roleID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || role.TenantID != tenant.ID {
		app.notFound(w, r)
		return
	}

	err = response.JSON(w, http.StatusOK, role)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteRole(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)
	roleID := chi.URLParam(r, "role_id")

	role, found, err := app.db.GetWorkspaceRole(roleID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || role.TenantID != tenant.ID {
		app.notFound(w, r)
		return
	}

	err = app.enforcer.DeleteCustomRole(tenant.Slug, role.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.db.DeleteWorkspaceRole(role.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
