package web

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
	roleID, err := app.enforcer.CreateRole(r.Context(), org.ID, input.WorkspaceID, input.Name, input.Description, input.ScopeType, input.Permissions)
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

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) roleInUse(w http.ResponseWriter, r *http.Request, err error) {
	var roleInUse access.RoleInUseError
	bindingCount := 0
	if errors.As(err, &roleInUse) {
		bindingCount = roleInUse.BindingCount
	}
	if jsonErr := response.JSON(w, http.StatusConflict, map[string]any{
		"error":         "Role is still used by policy bindings. Remove those policy bindings before deleting the role.",
		"binding_count": bindingCount,
	}); jsonErr != nil {
		app.serverError(w, r, jsonErr)
	}
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

	if ok, err := app.policySubjectExists(r, org.ID, input.SubjectType, input.SubjectID); err != nil {
		app.serverError(w, r, err)
		return
	} else if !ok {
		app.notFound(w, r)
		return
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
	if role.ScopeType != "org" || role.WorkspaceID != nil {
		v := validator.Validator{}
		v.AddFieldError("role_id", "Role scope must match resource type.")
		app.failedValidation(w, r, v)
		return
	}
	if !app.canManageProtectedOrgPolicy(r, org.ID, grantor.ID, role) {
		app.protectedOrgPolicyNotPermitted(w, r)
		return
	}
	if err := app.enforcer.BindRole(r.Context(), org.ID, input.RoleID, input.SubjectType, input.SubjectID, "org", org.ID, grantor.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

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

	rb, found, err := app.db.GetRoleBinding(r.Context(), bindingID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || rb.ResourceType != "org" || rb.ResourceID != org.ID {
		app.notFound(w, r)
		return
	}
	role, found, err := app.db.GetRole(r.Context(), rb.RoleID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if !app.canManageProtectedOrgPolicy(r, org.ID, grantor.ID, role) {
		app.protectedOrgPolicyNotPermitted(w, r)
		return
	}
	if isLastOwnerPolicy, checkErr := app.isLastOrgOwnerPolicy(r, org.ID, rb, role); checkErr != nil {
		app.serverError(w, r, checkErr)
		return
	} else if isLastOwnerPolicy {
		v := validator.Validator{}
		v.AddError("Cannot revoke the last owner policy of an organization.")
		app.failedValidation(w, r, v)
		return
	}
	if err = app.enforcer.UnbindRole(r.Context(), bindingID, org.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

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

func (app *application) canManageProtectedOrgPolicy(r *http.Request, orgID, grantorID int64, role database.Role) bool {
	for _, permission := range protectedOrgPolicyPermissions(role) {
		if !app.enforcer.Can(r.Context(), grantorID, orgID, "org", "org", orgID, permission) {
			return false
		}
	}
	return true
}

func (app *application) protectedOrgPolicyNotPermitted(w http.ResponseWriter, r *http.Request) {
	app.errorMessage(w, r, http.StatusForbidden, "Only users who already have organization deletion or ownership transfer permission can manage policies that grant those permissions.", nil)
}

func protectedOrgPolicyPermissions(role database.Role) []string {
	protected := make([]string, 0, 2)
	seen := map[string]bool{}
	add := func(permission string) {
		if !seen[permission] {
			protected = append(protected, permission)
			seen[permission] = true
		}
	}
	for _, permission := range role.Permissions {
		switch permission {
		case access.PermOrgDelete, access.PermOrgTransferOwnership:
			add(permission)
		}
	}
	if role.IsBuiltin && role.Name == access.BuiltinOrgOwnerRole && role.ScopeType == "org" && role.WorkspaceID == nil {
		add(access.PermOrgDelete)
		add(access.PermOrgTransferOwnership)
	}
	return protected
}

func (app *application) isLastOrgOwnerPolicy(r *http.Request, orgID int64, binding database.RoleBinding, role database.Role) (bool, error) {
	if binding.ResourceType != "org" || binding.ResourceID != orgID {
		return false, nil
	}

	if !role.IsBuiltin || role.Name != access.BuiltinOrgOwnerRole || role.ScopeType != "org" || role.WorkspaceID != nil {
		return false, nil
	}

	count, err := app.db.CountRoleBindings(r.Context(), orgID, binding.RoleID, "org", orgID)
	if err != nil {
		return false, err
	}
	return count <= 1, nil
}

func (app *application) policySubjectExists(r *http.Request, orgID int64, subjectType string, subjectID int64) (bool, error) {
	switch subjectType {
	case access.SubjectTypeAccount:
		_, found, err := app.db.GetAccount(r.Context(), subjectID)
		if err != nil || !found {
			return found, err
		}
		return app.db.IsOrgMember(r.Context(), orgID, subjectID)
	case access.SubjectTypeTeam:
		team, found, err := app.db.GetTeamByID(r.Context(), subjectID)
		if err != nil || !found {
			return found, err
		}
		return team.OrgID == orgID, nil
	case access.SubjectTypeOrgMembers:
		return subjectID == orgID, nil
	default:
		return false, nil
	}
}

func validPolicySubjectType(subjectType string) bool {
	switch subjectType {
	case access.SubjectTypeAccount, access.SubjectTypeTeam, access.SubjectTypeOrgMembers:
		return true
	default:
		return false
	}
}
