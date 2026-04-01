package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	wss, err := app.db.ListWorkspacesByOwner("org", org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, wss)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createWorkspace(w http.ResponseWriter, r *http.Request) {
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
	ws, err := app.db.InsertWorkspace(&org.ID, "org", org.ID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	err := response.JSON(w, http.StatusOK, ws)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateWorkspace(w http.ResponseWriter, r *http.Request) {
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

	ws := contextGetWorkspace(r)
	err = app.db.UpdateWorkspace(ws.ID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	err := app.db.DeleteWorkspace(ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidateAncestry("workspace", ws.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listWorkspaceBindings(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	rbs, err := app.db.ListRoleBindings(org.ID, "workspace", ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pbs, err := app.db.ListPermissionBindings(org.ID, "workspace", ws.ID)
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

func (app *application) grantWorkspaceRoleBinding(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleID      int64               `json:"role_id"`
		SubjectType string              `json:"subject_type"`
		SubjectID   int64               `json:"subject_id"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.RoleID > 0, "role_id", "role_id is required")
	input.V.CheckField(input.SubjectType == "account" || input.SubjectType == "team", "subject_type", "must be account or team")
	input.V.CheckField(input.SubjectID > 0, "subject_id", "subject_id is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	grantor := contextGetAccount(r)

	err = app.enforcer.BindRole(r.Context(), org.ID, input.RoleID, input.SubjectType, input.SubjectID, "workspace", ws.ID, grantor.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeWorkspaceBinding(w http.ResponseWriter, r *http.Request) {
	bindingIDStr := chi.URLParam(r, "binding_id")
	bindingID, err := strconv.ParseInt(bindingIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	err = app.enforcer.UnbindRole(r.Context(), bindingID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
