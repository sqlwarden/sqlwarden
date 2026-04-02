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

func (app *application) listWorkspaceRoles(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	roles, err := app.db.ListWorkspaceRoles(r.Context(), org.ID, ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, roles)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		Permissions []string            `json:"permissions"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	for _, p := range input.Permissions {
		input.V.CheckField(access.ValidForScope(p, "workspace"), "permissions", p+" is not valid for workspace scope")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	roleID, err := app.enforcer.CreateRole(r.Context(), org.ID, &ws.ID, input.Name, input.Description, "workspace", input.Permissions)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	role, found, err := app.db.GetRole(r.Context(), roleID, org.ID)
	if err != nil || !found {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, role)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	roleIDStr := chi.URLParam(r, "role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	role, found, err := app.db.GetRole(r.Context(), roleID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || role.WorkspaceID == nil || *role.WorkspaceID != ws.ID {
		app.notFound(w, r)
		return
	}

	err = response.JSON(w, http.StatusOK, role)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	roleIDStr := chi.URLParam(r, "role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	role, found, err := app.db.GetRole(r.Context(), roleID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || role.WorkspaceID == nil || *role.WorkspaceID != ws.ID {
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

func (app *application) listWorkspacePermissions(w http.ResponseWriter, r *http.Request) {
	err := response.JSON(w, http.StatusOK, map[string]any{
		"permissions": access.ScopePermissions["workspace"],
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
