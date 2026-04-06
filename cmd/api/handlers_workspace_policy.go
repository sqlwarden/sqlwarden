package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listWorkspaceRoles(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if _, ok := r.URL.Query()["sort"]; !ok {
		q.Sort = "name"
	}
	if _, ok := r.URL.Query()["order"]; !ok {
		q.Order = "asc"
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	var builtin *bool
	if raw := strings.TrimSpace(r.URL.Query().Get("builtin")); raw != "" {
		switch raw {
		case "true":
			v := true
			builtin = &v
		case "false":
			v := false
			builtin = &v
		default:
			app.failedValidation(w, r, fieldErrors(map[string]string{"builtin": "must be true or false"}))
			return
		}
	}

	roles, err := app.db.ListWorkspaceRolesPage(r.Context(), database.ListRolesParams{
		OrgID:       org.ID,
		WorkspaceID: &ws.ID,
		Search:      q.Search,
		Name:        strings.TrimSpace(r.URL.Query().Get("name")),
		IsBuiltin:   builtin,
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	})
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
		if errors.Is(err, access.ErrInvalidScopePermission) || errors.Is(err, access.ErrUnknownPermission) {
			input.V.AddFieldError("permissions", err.Error())
			app.failedValidation(w, r, input.V)
			return
		}
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "a role with this name already exists in this workspace")
			return
		}
		app.serverError(w, r, err)
		return
	}

	role, found, err := app.db.GetRole(r.Context(), roleID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
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
		if errors.Is(err, access.ErrBuiltinRole) {
			app.notPermitted(w, r)
			return
		}
		if errors.Is(err, access.ErrRoleNotFound) {
			app.notFound(w, r)
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

// listWorkspacePolicies returns all role bindings for the workspace
// and every resource inside it (environments, connections).
func (app *application) listWorkspacePolicies(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at":    "created_at",
		"subject_name":  "subject_name",
		"resource_name": "resource_name",
		"permission":    "permission",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	subjectID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("subject_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			errs["subject_id"] = "must be a positive integer"
		} else {
			subjectID = parsed
		}
	}
	resourceID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("resource_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			errs["resource_id"] = "must be a positive integer"
		} else {
			resourceID = parsed
		}
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	result, err := app.db.ListWorkspacePoliciesPage(r.Context(), database.ListWorkspacePoliciesParams{
		OrgID:        org.ID,
		WorkspaceID:  ws.ID,
		Search:       q.Search,
		SubjectID:    subjectID,
		SubjectType:  strings.TrimSpace(r.URL.Query().Get("subject_type")),
		Permission:   strings.TrimSpace(r.URL.Query().Get("permission")),
		ResourceID:   resourceID,
		ResourceType: strings.TrimSpace(r.URL.Query().Get("resource_type")),
		Sort:         q.Sort,
		Order:        q.Order,
		Page:         q.Page,
		PageSize:     q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, result)
	if err != nil {
		app.serverError(w, r, err)
	}
}

// grantWorkspacePolicy creates a role binding for a resource within
// the workspace. resource_type defaults to "workspace"; for "environment" or
// "connection" a resource_id must be supplied and is validated for ownership.
func (app *application) grantWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleID       int64               `json:"role_id"`
		SubjectType  string              `json:"subject_type"`
		SubjectID    int64               `json:"subject_id"`
		ResourceType string              `json:"resource_type"`
		ResourceID   int64               `json:"resource_id"`
		V            validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	if input.ResourceType == "" {
		input.ResourceType = "workspace"
	}

	input.V.CheckField(input.RoleID > 0, "role_id", "role_id is required")
	input.V.CheckField(input.SubjectType == "account" || input.SubjectType == "team", "subject_type", "must be account or team")
	input.V.CheckField(input.SubjectID > 0, "subject_id", "subject_id is required")
	validTypes := map[string]bool{"workspace": true, "environment": true, "connection": true}
	input.V.CheckField(validTypes[input.ResourceType], "resource_type", "must be workspace, environment, or connection")
	if input.ResourceType != "workspace" {
		input.V.CheckField(input.ResourceID > 0, "resource_id", "resource_id is required for non-workspace resources")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	grantor := contextGetAccount(r)

	if ok, err := app.policySubjectExists(r, org.ID, input.SubjectType, input.SubjectID); err != nil {
		app.serverError(w, r, err)
		return
	} else if !ok {
		app.notFound(w, r)
		return
	}

	// Resolve and validate the target resource belongs to this workspace.
	var resourceID int64
	switch input.ResourceType {
	case "workspace":
		resourceID = ws.ID
	case "environment":
		env, found, err := app.db.GetEnvironment(r.Context(), input.ResourceID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found || env.WorkspaceID != ws.ID {
			app.notFound(w, r)
			return
		}
		resourceID = env.ID
	case "connection":
		conn, found, err := app.db.GetConnection(r.Context(), input.ResourceID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found || conn.WorkspaceID != ws.ID {
			app.notFound(w, r)
			return
		}
		resourceID = conn.ID
	}

	role, found, err := app.db.GetRole(r.Context(), input.RoleID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	switch input.ResourceType {
	case "workspace":
		if role.ScopeType != "workspace" {
			v := validator.Validator{}
			v.AddFieldError("role_id", "role scope must match resource type")
			app.failedValidation(w, r, v)
			return
		}
		if role.WorkspaceID != nil && *role.WorkspaceID != ws.ID {
			app.notFound(w, r)
			return
		}
	default:
		if role.ScopeType != input.ResourceType {
			v := validator.Validator{}
			v.AddFieldError("role_id", "role scope must match resource type")
			app.failedValidation(w, r, v)
			return
		}
	}
	if err := app.enforcer.BindRole(r.Context(), org.ID, input.RoleID, input.SubjectType, input.SubjectID, input.ResourceType, resourceID, grantor.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// revokeWorkspacePolicy removes a role binding. It verifies the
// binding's resource belongs to this workspace before revoking.
func (app *application) revokeWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	bindingIDStr := chi.URLParam(r, "binding_id")
	bindingID, err := strconv.ParseInt(bindingIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)

	rb, found, err := app.db.GetRoleBinding(r.Context(), bindingID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if ok, err := app.resourceBelongsToWorkspace(r, rb.ResourceType, rb.ResourceID, ws.ID); err != nil {
		app.serverError(w, r, err)
		return
	} else if !ok {
		app.notFound(w, r)
		return
	}
	if err = app.enforcer.UnbindRole(r.Context(), bindingID, org.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// resourceBelongsToWorkspace validates that the given resource is owned by wsID.
func (app *application) resourceBelongsToWorkspace(r *http.Request, resourceType string, resourceID, wsID int64) (bool, error) {
	switch resourceType {
	case "workspace":
		return resourceID == wsID, nil
	case "environment":
		env, found, err := app.db.GetEnvironment(r.Context(), resourceID)
		if err != nil {
			return false, err
		}
		return found && env.WorkspaceID == wsID, nil
	case "connection":
		conn, found, err := app.db.GetConnection(r.Context(), resourceID)
		if err != nil {
			return false, err
		}
		return found && conn.WorkspaceID == wsID, nil
	default:
		return false, nil
	}
}
