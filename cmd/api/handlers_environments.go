package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listEnvironments(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	envs, err := app.db.ListEnvironments(ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, envs)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createEnvironment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	env, err := app.db.InsertEnvironment(ws.ID, &org.ID, ws.OwnerType, ws.OwnerID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEnvironment(w http.ResponseWriter, r *http.Request) {
	env := contextGetEnvironment(r)
	err := response.JSON(w, http.StatusOK, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	env := contextGetEnvironment(r)
	err = app.db.UpdateEnvironment(env.ID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	env := contextGetEnvironment(r)
	err := app.db.DeleteEnvironment(env.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidateAncestry("environment", env.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listEnvironmentBindings(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	env := contextGetEnvironment(r)

	rbs, err := app.db.ListRoleBindings(org.ID, "environment", env.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pbs, err := app.db.ListPermissionBindings(org.ID, "environment", env.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, map[string]any{
		"role_bindings":       rbs,
		"permission_bindings": pbs,
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) grantEnvironmentAccess(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleID      int64               `json:"role_id"`
		Permission  string              `json:"permission"`
		SubjectType string              `json:"subject_type"`
		SubjectID   int64               `json:"subject_id"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	hasRole := input.RoleID > 0
	hasPerm := input.Permission != ""
	input.V.CheckField(hasRole || hasPerm, "role_id", "one of role_id or permission is required")
	input.V.CheckField(!(hasRole && hasPerm), "role_id", "only one of role_id or permission may be set")
	input.V.CheckField(input.SubjectType == "account" || input.SubjectType == "team", "subject_type", "must be account or team")
	input.V.CheckField(input.SubjectID > 0, "subject_id", "subject_id is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	env := contextGetEnvironment(r)
	grantor := contextGetAccount(r)

	if hasRole {
		err = app.enforcer.BindRole(r.Context(), org.ID, input.RoleID, input.SubjectType, input.SubjectID, "environment", env.ID, grantor.ID)
	} else {
		err = app.enforcer.GrantPermission(r.Context(), org.ID, input.Permission, input.SubjectType, input.SubjectID, "environment", env.ID, grantor.ID)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeEnvironmentAccess(w http.ResponseWriter, r *http.Request) {
	bindingIDStr := chi.URLParam(r, "binding_id")
	bindingID, err := strconv.ParseInt(bindingIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	kind := r.URL.Query().Get("kind")

	if kind == "permission" {
		err = app.enforcer.RevokePermission(r.Context(), bindingID, org.ID)
	} else {
		err = app.enforcer.UnbindRole(r.Context(), bindingID, org.ID)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
