package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/rbac"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// rbacError translates rbac service failures to the standard error envelope.
func (app *application) rbacError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *rbac.ValidationError
	switch {
	case errors.As(err, &validation):
		app.failedValidation(w, r, fieldErrors(validation.Fields))
	case errors.Is(err, rbac.ErrLastOwner):
		v := validator.Validator{}
		v.AddError("Cannot revoke the last owner policy of an organization.")
		app.failedValidation(w, r, v)
	case errors.Is(err, rbac.ErrPrivilegedPolicy):
		app.errorMessage(w, r, http.StatusForbidden, "Only users who already have organization deletion or ownership transfer permission can manage policies that grant those permissions.", nil)
	case errors.Is(err, rbac.ErrBuiltinRole):
		app.notPermitted(w, r)
	case errors.Is(err, rbac.ErrRoleInUse):
		app.roleInUse(w, r, err)
	default:
		app.serviceError(w, r, err)
	}
}

func (app *application) listRoles(w http.ResponseWriter, r *http.Request) {
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

	roles, err := app.services.RBAC.ListRoles(r.Context(), principal, contextGetPathIDs(r).OrgID, rbac.ListRolesInput{
		Scope:    r.URL.Query().Get("scope"),
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

func (app *application) createRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ScopeType   string   `json:"scope_type"`
		WorkspaceID *int64   `json:"workspace_id"`
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

	role, err := app.services.RBAC.CreateRole(r.Context(), principal, contextGetPathIDs(r).OrgID, rbac.CreateRoleInput{
		Name:        input.Name,
		Description: input.Description,
		ScopeType:   input.ScopeType,
		WorkspaceID: input.WorkspaceID,
		Permissions: input.Permissions,
	})
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "organization role created", slog.Int64("role_id", role.ID), slog.String("scope_type", role.ScopeType), slog.Int("permission_count", len(input.Permissions)))
	if err := response.JSON(w, http.StatusCreated, role); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := strconv.ParseInt(chi.URLParam(r, "role_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	role, err := app.services.RBAC.GetRole(r.Context(), principal, contextGetPathIDs(r).OrgID, roleID)
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, role); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateRole(w http.ResponseWriter, r *http.Request) {
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

	role, err := app.services.RBAC.UpdateRole(r.Context(), principal, contextGetPathIDs(r).OrgID, roleID, rbac.UpdateRoleInput{
		Name:        input.Name,
		Description: input.Description,
		Permissions: input.Permissions,
	})
	var refusal *rbac.RefusalError
	if errors.As(err, &refusal) && errors.Is(err, rbac.ErrPrivilegedPolicy) {
		app.logWarn(r, "protected organization role update blocked", slog.Int64("role_id", refusal.Role.ID), slog.String("role_name", refusal.Role.Name))
	}
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "organization role updated", slog.Int64("role_id", role.ID), slog.Int("permission_count", len(input.Permissions)))
	if err := response.JSON(w, http.StatusOK, role); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteRole(w http.ResponseWriter, r *http.Request) {
	roleID, err := strconv.ParseInt(chi.URLParam(r, "role_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	if err := app.services.RBAC.DeleteRole(r.Context(), principal, contextGetPathIDs(r).OrgID, roleID); err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "organization role deleted", slog.Int64("role_id", roleID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) roleInUse(w http.ResponseWriter, r *http.Request, err error) {
	var roleInUse access.RoleInUseError
	bindingCount := 0
	if errors.As(err, &roleInUse) {
		bindingCount = roleInUse.BindingCount
	}
	app.apiError(w, r, http.StatusConflict, apiErrorResourceInUse, "Role is still used by policy bindings. Remove those policy bindings before deleting the role.", response.APIError{
		Details: map[string]any{"binding_count": bindingCount},
	}, nil)
}

func (app *application) listOrgPolicies(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at":   "created_at",
		"subject_name": "subject_name",
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
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	result, err := app.services.RBAC.ListPolicies(r.Context(), principal, contextGetPathIDs(r).OrgID, rbac.ListPoliciesParams{
		Search:      q.Search,
		SubjectID:   subjectID,
		SubjectType: strings.TrimSpace(r.URL.Query().Get("subject_type")),
		Permission:  strings.TrimSpace(r.URL.Query().Get("permission")),
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	})
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getOrgPolicy(w http.ResponseWriter, r *http.Request) {
	bindingID, err := strconv.ParseInt(chi.URLParam(r, "binding_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	item, err := app.services.RBAC.GetPolicy(r.Context(), principal, contextGetPathIDs(r).OrgID, bindingID)
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, item); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) grantOrgPolicy(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RoleID      int64  `json:"role_id"`
		SubjectType string `json:"subject_type"`
		SubjectID   int64  `json:"subject_id"`
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

	err := app.services.RBAC.GrantPolicy(r.Context(), principal, contextGetPathIDs(r).OrgID, rbac.GrantPolicyInput{
		RoleID:      input.RoleID,
		SubjectType: input.SubjectType,
		SubjectID:   input.SubjectID,
	})
	var refusal *rbac.RefusalError
	if errors.As(err, &refusal) && errors.Is(err, rbac.ErrPrivilegedPolicy) {
		app.logWarn(r, "protected organization policy grant blocked", slog.Int64("role_id", refusal.Role.ID), slog.String("role_name", refusal.Role.Name), slog.String("subject_type", input.SubjectType), slog.Int64("subject_id", input.SubjectID))
	}
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "organization policy granted", slog.Int64("role_id", input.RoleID), slog.String("subject_type", input.SubjectType), slog.Int64("subject_id", input.SubjectID), slog.String("resource_type", "org"), slog.Int64("resource_id", ids.OrgID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeOrgPolicy(w http.ResponseWriter, r *http.Request) {
	bindingID, err := strconv.ParseInt(chi.URLParam(r, "binding_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	rb, err := app.services.RBAC.RevokePolicy(r.Context(), principal, contextGetPathIDs(r).OrgID, bindingID)
	var refusal *rbac.RefusalError
	if errors.As(err, &refusal) {
		b := refusal.Binding
		if errors.Is(err, rbac.ErrLastOwner) {
			app.logWarn(r, "last organization owner policy revoke blocked", slog.Int64("binding_id", bindingID), slog.Int64("role_id", refusal.Role.ID), slog.String("subject_type", b.SubjectType), slog.Int64("subject_id", b.SubjectID))
		} else {
			app.logWarn(r, "protected organization policy revoke blocked", slog.Int64("binding_id", bindingID), slog.Int64("role_id", refusal.Role.ID), slog.String("role_name", refusal.Role.Name), slog.String("subject_type", b.SubjectType), slog.Int64("subject_id", b.SubjectID))
		}
	}
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	app.logInfo(r, "organization policy revoked", slog.Int64("binding_id", bindingID), slog.Int64("role_id", rb.RoleID), slog.String("subject_type", rb.SubjectType), slog.Int64("subject_id", rb.SubjectID), slog.String("resource_type", rb.ResourceType), slog.Int64("resource_id", rb.ResourceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listPermissions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	catalog, err := app.services.RBAC.ListPermissions(r.Context(), principal, contextGetPathIDs(r).OrgID)
	if err != nil {
		app.rbacError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, catalog); err != nil {
		app.serverError(w, r, err)
	}
}
