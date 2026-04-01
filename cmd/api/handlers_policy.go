package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listRoles(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	roles, err := app.db.ListRoles(org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, roles)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		ScopeType   string              `json:"scope_type"`
		Permissions []string            `json:"permissions"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.ScopeType != "", "scope_type", "scope_type is required")
	validScopes := map[string]bool{"org": true, "workspace": true, "environment": true, "connection": true}
	input.V.CheckField(validScopes[input.ScopeType], "scope_type", "must be org, workspace, environment, or connection")
	for _, p := range input.Permissions {
		input.V.CheckField(access.ValidForScope(p, input.ScopeType), "permissions", p+" is not valid for scope "+input.ScopeType)
	}

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	roleID, err := app.enforcer.CreateRole(r.Context(), org.ID, input.Name, input.Description, input.ScopeType, input.Permissions)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	role, found, err := app.db.GetRole(roleID, org.ID)
	if err != nil || !found {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, role)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getRole(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	roleIDStr := chi.URLParam(r, "role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	role, found, err := app.db.GetRole(roleID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = response.JSON(w, http.StatusOK, role)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteRole(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	roleIDStr := chi.URLParam(r, "role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	err = app.enforcer.DeleteRole(r.Context(), roleID, org.ID)
	if err != nil {
		if err.Error() == "cannot delete a builtin role" {
			app.notPermitted(w, r)
			return
		}
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listPermissions(w http.ResponseWriter, r *http.Request) {
	err := response.JSON(w, http.StatusOK, map[string]any{
		"permissions": access.AllPermissions(),
		"scope_map":   access.ScopePermissions,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
