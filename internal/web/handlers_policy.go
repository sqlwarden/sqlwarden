package web

import (
	"errors"
	"log/slog"
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

func (app *application) listRoles(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

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
			app.failedValidation(w, r, fieldErrors(map[string]string{"builtin": "Built-in flag must be true or false."}))
			return
		}
	}

	scope := "all"
	if raw := strings.TrimSpace(r.URL.Query().Get("scope")); raw != "" {
		switch raw {
		case "all", "org", "workspace":
			scope = raw
		default:
			app.failedValidation(w, r, fieldErrors(map[string]string{"scope": "Scope must be all, org, or workspace."}))
			return
		}
	}

	roles, err := app.db.ListRolesPage(r.Context(), database.ListRolesParams{
		OrgID:     org.ID,
		Scope:     scope,
		Search:    q.Search,
		Name:      strings.TrimSpace(r.URL.Query().Get("name")),
		IsBuiltin: builtin,
		Sort:      q.Sort,
		Order:     q.Order,
		Page:      q.Page,
		PageSize:  q.PageSize,
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

func (app *application) createRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		ScopeType   string              `json:"scope_type"`
		WorkspaceID *int64              `json:"workspace_id"`
		Permissions []string            `json:"permissions"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	if input.ScopeType == "" {
		input.ScopeType = "org"
	}
	input.V.CheckField(input.ScopeType == "org", "scope_type", "Organization roles must have scope_type=org.")
	if input.WorkspaceID != nil {
		input.V.AddFieldError("workspace_id", "Organization roles cannot set workspace_id.")
	}
	for _, p := range input.Permissions {
		input.V.CheckField(access.ValidForScope(p, input.ScopeType), "permissions", "Permission "+p+" is not valid for scope "+input.ScopeType+".")
	}

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	actor := contextGetAccount(r)
	roleID, err := app.accessService.CreateOrgRole(r.Context(), access.OrgRoleInput{
		OrgID: org.ID, ActorID: actor.ID, Name: input.Name, Description: input.Description,
		ScopeType: input.ScopeType, Permissions: input.Permissions,
	})
	if err != nil {
		if errors.Is(err, access.ErrInvalidScopePermission) || errors.Is(err, access.ErrUnknownPermission) {
			input.V.AddFieldError("permissions", "Permissions include a permission that is not valid for this scope.")
			app.failedValidation(w, r, input.V)
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

	app.logInfo(r, "organization role created", slog.Int64("role_id", role.ID), slog.String("scope_type", role.ScopeType), slog.Int("permission_count", len(input.Permissions)))
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

	role, found, err := app.db.GetRole(r.Context(), roleID, org.ID)
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

func (app *application) updateRole(w http.ResponseWriter, r *http.Request) {
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

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	roleIDStr := chi.URLParam(r, "role_id")
	roleID, err := strconv.ParseInt(roleIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	err = app.accessService.UpdateOrgRole(r.Context(), access.UpdateOrgRoleInput{
		OrgID: org.ID, RoleID: roleID, ActorID: contextGetAccount(r).ID,
		Name: input.Name, Description: input.Description, Permissions: input.Permissions,
	})
	if err != nil {
		if errors.Is(err, access.ErrBuiltinRole) {
			app.notPermitted(w, r)
			return
		}
		if errors.Is(err, access.ErrRoleNotFound) {
			app.notFound(w, r)
			return
		}
		if errors.Is(err, access.ErrInvalidScopePermission) || errors.Is(err, access.ErrUnknownPermission) {
			input.V.AddFieldError("permissions", "Permissions include a permission that is not valid for this scope.")
			app.failedValidation(w, r, input.V)
			return
		}
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A role with this name already exists in this organization.")
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

	app.logInfo(r, "organization role updated", slog.Int64("role_id", role.ID), slog.Int("permission_count", len(input.Permissions)))
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

	err = app.accessService.DeleteOrgRole(r.Context(), access.DeleteOrgRoleInput{
		OrgID: org.ID, RoleID: roleID, ActorID: contextGetAccount(r).ID,
	})
	if err != nil {
		if errors.Is(err, access.ErrBuiltinRole) {
			app.notPermitted(w, r)
			return
		}
		if errors.Is(err, access.ErrRoleNotFound) {
			app.notFound(w, r)
			return
		}
		if errors.Is(err, access.ErrRoleInUse) {
			app.roleInUse(w, r, err)
			return
		}
		app.serverError(w, r, err)
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
	org := contextGetOrg(r)

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

	result, err := app.db.ListOrgPoliciesPage(r.Context(), database.ListOrgPoliciesParams{
		OrgID:       org.ID,
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
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, result)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getOrgPolicy(w http.ResponseWriter, r *http.Request) {
	bindingIDStr := chi.URLParam(r, "binding_id")
	bindingID, err := strconv.ParseInt(bindingIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)

	binding, err := app.accessService.OrgPolicyBinding(r.Context(), org.ID, bindingID)
	if errors.Is(err, access.ErrRoleBindingNotFound) {
		app.notFound(w, r)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	item, err := app.db.GetPolicyBindingItem(r.Context(), org.ID, databaseRoleBinding(binding))
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, item)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func databaseRoleBinding(binding access.RoleBinding) database.RoleBinding {
	return database.RoleBinding{
		ID: binding.ID, OrgID: binding.OrgID, RoleID: binding.RoleID,
		SubjectType: binding.SubjectType, SubjectID: binding.SubjectID,
		ResourceType: binding.ResourceType, ResourceID: binding.ResourceID,
		CreatedAt: binding.CreatedAt,
	}
}

func (app *application) grantOrgPolicy(w http.ResponseWriter, r *http.Request) {
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

	input.V.CheckField(input.RoleID > 0, "role_id", "Role is required.")
	input.V.CheckField(validPolicySubjectType(input.SubjectType), "subject_type", "Subject type must be account, team, or org_members.")
	input.V.CheckField(input.SubjectID > 0, "subject_id", "Subject is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	grantor := contextGetAccount(r)
	err = app.accessService.GrantOrgPolicy(r.Context(), access.GrantOrgPolicyInput{
		OrgID: org.ID, GrantorID: grantor.ID, RoleID: input.RoleID,
		SubjectType: input.SubjectType, SubjectID: input.SubjectID,
	})
	if errors.Is(err, access.ErrSubjectNotFound) || errors.Is(err, access.ErrRoleNotFound) {
		app.notFound(w, r)
		return
	}
	if errors.Is(err, access.ErrRoleScopeMismatch) {
		v := validator.Validator{}
		v.AddFieldError("role_id", "Role scope must match resource type.")
		app.failedValidation(w, r, v)
		return
	}
	if errors.Is(err, access.ErrProtectedPolicy) {
		app.logWarn(r, "protected organization policy grant blocked", slog.Int64("role_id", input.RoleID), slog.String("subject_type", input.SubjectType), slog.Int64("subject_id", input.SubjectID))
		app.protectedOrgPolicyNotPermitted(w, r)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "organization policy granted", slog.Int64("role_id", input.RoleID), slog.String("subject_type", input.SubjectType), slog.Int64("subject_id", input.SubjectID), slog.String("resource_type", "org"), slog.Int64("resource_id", org.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeOrgPolicy(w http.ResponseWriter, r *http.Request) {
	bindingIDStr := chi.URLParam(r, "binding_id")
	bindingID, err := strconv.ParseInt(bindingIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	grantor := contextGetAccount(r)

	rb, err := app.accessService.RevokeOrgPolicy(r.Context(), access.RevokeOrgPolicyInput{
		OrgID: org.ID, GrantorID: grantor.ID, BindingID: bindingID,
	})
	if errors.Is(err, access.ErrRoleBindingNotFound) || errors.Is(err, access.ErrRoleNotFound) {
		app.notFound(w, r)
		return
	}
	if errors.Is(err, access.ErrProtectedPolicy) {
		app.logWarn(r, "protected organization policy revoke blocked", slog.Int64("binding_id", bindingID), slog.Int64("role_id", rb.RoleID))
		app.protectedOrgPolicyNotPermitted(w, r)
		return
	}
	if errors.Is(err, access.ErrLastOwnerPolicy) {
		app.logWarn(r, "last organization owner policy revoke blocked", slog.Int64("binding_id", bindingID), slog.Int64("role_id", rb.RoleID))
		v := validator.Validator{}
		v.AddError("Cannot revoke the last owner policy of an organization.")
		app.failedValidation(w, r, v)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "organization policy revoked", slog.Int64("binding_id", bindingID), slog.Int64("role_id", rb.RoleID), slog.String("subject_type", rb.SubjectType), slog.Int64("subject_id", rb.SubjectID), slog.String("resource_type", rb.ResourceType), slog.Int64("resource_id", rb.ResourceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listPermissions(w http.ResponseWriter, r *http.Request) {
	err := response.JSON(w, http.StatusOK, map[string]any{
		"permissions":        access.AllPermissions(),
		"permission_details": access.AllPermissionDefinitions(),
		"scope_map":          access.ScopePermissions,
		"scope_details":      access.ScopePermissionDefinitionMap(),
		"resource_map":       access.ResourcePermissions,
		"resource_details":   access.ResourcePermissionDefinitionMap(),
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) protectedOrgPolicyNotPermitted(w http.ResponseWriter, r *http.Request) {
	app.errorMessage(w, r, http.StatusForbidden, "Only users who already have organization deletion or ownership transfer permission can manage policies that grant those permissions.", nil)
}

func validPolicySubjectType(subjectType string) bool {
	switch subjectType {
	case access.SubjectTypeAccount, access.SubjectTypeTeam, access.SubjectTypeOrgMembers:
		return true
	default:
		return false
	}
}
