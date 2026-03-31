package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)

	allWS, err := app.db.GetWorkspacesByTenant(org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// Admin/owner sees all workspaces.
	if app.enforcer.Can(account.ID, org.Slug, "*", "*") {
		if allWS == nil {
			allWS = []database.Workspace{}
		}
		err = response.JSON(w, http.StatusOK, allWS)
		if err != nil {
			app.serverError(w, r, err)
		}
		return
	}

	// Non-admin: filter to workspaces the user can connect to.
	var visible []database.Workspace
	for _, ws := range allWS {
		if app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "connect") {
			visible = append(visible, ws)
		}
	}

	if visible == nil {
		visible = []database.Workspace{}
	}

	err = response.JSON(w, http.StatusOK, visible)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createWorkspace(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

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

	ws, err := app.db.InsertWorkspace(org.ID, input.Name, input.Description)
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
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	name := ws.Name
	description := ws.Description

	if input.Name != nil {
		name = *input.Name
	}
	if input.Description != nil {
		description = *input.Description
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(name), "name", "Name is required")
	v.CheckField(validator.MaxRunes(name, 100), "name", "Must not be more than 100 characters")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	err = app.db.UpdateWorkspace(ws.ID, name, description)
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

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	entries, err := app.enforcer.ListWorkspaceAccess(org.Slug, ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, entries)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) grantWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		Subject   string     `json:"subject"`
		Action    string     `json:"action"`
		ExpiresAt *time.Time `json:"expires_at"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.Subject), "subject", "Subject is required")
	v.CheckField(validator.In(input.Action, "connect", "query", "execute", "manage"), "action", "Action must be connect, query, execute, or manage")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	// Resolve subject:
	// "account:<id>" -> bare account ID (Casbin uses account ID directly)
	// "team:<slug>" -> "team:<id>" (Casbin uses "team:<id>" as group role)
	resolvedSubject := input.Subject
	if id, ok := strings.CutPrefix(input.Subject, "account:"); ok {
		resolvedSubject = id
	} else if teamSlug, ok := strings.CutPrefix(input.Subject, "team:"); ok {
		team, found, err := app.db.GetTeamBySlug(org.ID, teamSlug)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.notFound(w, r)
			return
		}
		resolvedSubject = "team:" + team.ID
	}

	err = app.enforcer.GrantWorkspaceAccess(resolvedSubject, org.Slug, ws.ID, input.Action)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	_, err = app.db.InsertAccessGrant(org.ID, resolvedSubject, "workspace:"+ws.ID, input.Action, account.ID, input.ExpiresAt)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (app *application) revokeWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	subject := chi.URLParam(r, "subject")

	err := app.enforcer.RevokeWorkspaceAccess(subject, org.Slug, ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.db.DeleteAccessGrant(subject, "workspace:"+ws.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) applyWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	var input struct {
		RoleID  string `json:"role_id"`
		Subject string `json:"subject"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.RoleID), "role_id", "Role ID is required")
	v.CheckField(validator.NotBlank(input.Subject), "subject", "Subject is required")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	role, found, err := app.db.GetWorkspaceRole(input.RoleID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || role.TenantID != org.ID {
		app.notFound(w, r)
		return
	}

	err = app.enforcer.SeedCustomRole(org.Slug, role.ID, ws.ID, []string{"connect", "query", "execute", "manage"})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.AssignCustomRole(input.Subject, org.Slug, role.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeWorkspaceRoleAssignment(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	if !app.enforcer.Can(account.ID, org.Slug, "workspace:"+ws.ID, "manage") {
		app.notPermitted(w, r)
		return
	}

	roleID := chi.URLParam(r, "role_id")
	subject := chi.URLParam(r, "subject")

	err := app.enforcer.RevokeCustomRole(subject, org.Slug, roleID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
