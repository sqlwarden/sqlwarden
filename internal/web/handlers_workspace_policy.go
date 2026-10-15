package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listWorkspaceRoles(w http.ResponseWriter, r *http.Request) {
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
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	roles, err := app.services.RBAC.ListWorkspaceRoles(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, rbac.ListWorkspaceRolesInput{
		Builtin:  r.URL.Query().Get("builtin"),
		Name:     r.URL.Query().Get("name"),
		Search:   q.Search,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, roles); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ScopeType   string   `json:"scope_type"`
		Permissions []string `json:"permissions"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)

	role, err := app.services.RBAC.CreateWorkspaceRole(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, rbac.CreateWorkspaceRoleInput{
		Name:        input.Name,
		Description: input.Description,
		ScopeType:   input.ScopeType,
		Permissions: input.Permissions,
	})
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "workspace role created", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("role_id", role.ID), slog.String("scope_type", role.ScopeType), slog.Int("permission_count", len(input.Permissions)))
	if err := response.JSON(w, http.StatusCreated, role); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := strconv.ParseInt(chi.URLParam(r, "role_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	role, err := app.services.RBAC.GetWorkspaceRole(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, roleID)
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, role); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Permissions []string `json:"permissions"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	roleID, err := strconv.ParseInt(chi.URLParam(r, "role_id"), 10, 64)
	if err != nil {
		if input.Name == "" {
			app.failedValidation(w, r, fieldErrors(map[string]string{"name": "Name is required."}))
			return
		}
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)

	role, err := app.services.RBAC.UpdateWorkspaceRole(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, roleID, rbac.UpdateRoleInput{
		Name:        input.Name,
		Description: input.Description,
		Permissions: input.Permissions,
	})
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "workspace role updated", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("role_id", role.ID), slog.Int("permission_count", len(input.Permissions)))
	if err := response.JSON(w, http.StatusOK, role); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteWorkspaceRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := strconv.ParseInt(chi.URLParam(r, "role_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	if err := app.services.RBAC.DeleteWorkspaceRole(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, roleID); err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "workspace role deleted", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("role_id", roleID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listWorkspacePermissions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	catalog, err := app.services.RBAC.ListWorkspacePermissions(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID)
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, catalog); err != nil {
		app.serverError(w, r, err)
	}
}

// listWorkspacePolicies returns all role bindings for the workspace
// and every resource inside it (environments, connections).
func (app *application) listWorkspacePolicies(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at":    "created_at",
		"subject_name":  "subject_name",
		"resource_name": "resource_name",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	subjectID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("subject_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			errs["subject_id"] = "Subject must be a positive integer."
		} else {
			subjectID = parsed
		}
	}
	resourceID := int64(0)
	if raw := strings.TrimSpace(r.URL.Query().Get("resource_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 1 {
			errs["resource_id"] = "Resource must be a positive integer."
		} else {
			resourceID = parsed
		}
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	result, err := app.services.RBAC.ListWorkspacePolicies(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, rbac.ListWorkspacePoliciesParams{
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
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	bindingID, err := strconv.ParseInt(chi.URLParam(r, "binding_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	item, err := app.services.RBAC.GetWorkspacePolicy(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, bindingID)
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, item); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) grantWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleID       int64  `json:"role_id"`
		SubjectType  string `json:"subject_type"`
		SubjectID    int64  `json:"subject_id"`
		ResourceType string `json:"resource_type"`
		ResourceID   int64  `json:"resource_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)

	resourceID, err := app.services.RBAC.GrantWorkspacePolicy(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, rbac.GrantWorkspacePolicyInput{
		RoleID:       input.RoleID,
		SubjectType:  input.SubjectType,
		SubjectID:    input.SubjectID,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
	})
	var refusal *rbac.RefusalError
	if errors.As(err, &refusal) {
		app.logWarn(r, "protected workspace policy grant blocked", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("role_id", refusal.Role.ID), slog.String("subject_type", input.SubjectType), slog.Int64("subject_id", input.SubjectID))
	}
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	resourceType := input.ResourceType
	if resourceType == "" {
		resourceType = "workspace"
	}
	app.logInfo(r, "workspace policy granted", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("role_id", input.RoleID), slog.String("subject_type", input.SubjectType), slog.Int64("subject_id", input.SubjectID), slog.String("resource_type", resourceType), slog.Int64("resource_id", resourceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeWorkspacePolicy(w http.ResponseWriter, r *http.Request) {
	bindingID, err := strconv.ParseInt(chi.URLParam(r, "binding_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)

	rb, err := app.services.RBAC.RevokeWorkspacePolicy(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, bindingID)
	var refusal *rbac.RefusalError
	if errors.As(err, &refusal) {
		app.logWarn(r, "protected workspace policy revoke blocked", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("binding_id", bindingID), slog.Int64("role_id", refusal.Role.ID), slog.String("subject_type", refusal.Binding.SubjectType), slog.Int64("subject_id", refusal.Binding.SubjectID))
	}
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "workspace policy revoked", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("binding_id", bindingID), slog.Int64("role_id", rb.RoleID), slog.String("subject_type", rb.SubjectType), slog.Int64("subject_id", rb.SubjectID), slog.String("resource_type", rb.ResourceType), slog.Int64("resource_id", rb.ResourceID))
	w.WriteHeader(http.StatusNoContent)
}
