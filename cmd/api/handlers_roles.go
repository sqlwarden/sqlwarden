package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// WorkspaceRoleWithActions is a role enriched with its associated enforcer actions.
type WorkspaceRoleWithActions struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Actions     []string `json:"actions"`
}

func (app *application) listRoles(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	roles, err := app.db.GetWorkspaceRolesByTenant(tenant.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	result := make([]WorkspaceRoleWithActions, 0, len(roles))
	for _, role := range roles {
		actions := app.enforcer.ListRoleActions(role.ID)
		result = append(result, WorkspaceRoleWithActions{
			ID:          role.ID,
			TenantID:    role.TenantID,
			Name:        role.Name,
			Description: role.Description,
			Actions:     actions,
		})
	}

	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createRole(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	var input struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Actions     []string `json:"actions"`
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

	validActions := map[string]bool{"connect": true, "query": true, "execute": true, "manage": true}
	for _, action := range input.Actions {
		if !validActions[action] {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "invalid action: "+action, nil)
			return
		}
	}

	role, err := app.db.InsertWorkspaceRole(tenant.ID, input.Name, input.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	for _, action := range input.Actions {
		if err := app.enforcer.AddRoleAction(role.ID, tenant.Slug, action); err != nil {
			app.serverError(w, r, err)
			return
		}
	}

	result := WorkspaceRoleWithActions{
		ID:          role.ID,
		TenantID:    role.TenantID,
		Name:        role.Name,
		Description: role.Description,
		Actions:     input.Actions,
	}
	if err := response.JSON(w, http.StatusCreated, result); err != nil {
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
