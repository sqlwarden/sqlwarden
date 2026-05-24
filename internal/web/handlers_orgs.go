package web

import (
	"context"
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

const (
	singleUserDefaultOrgName = "Local"
	singleUserDefaultOrgSlug = "local"
)

func (app *application) createOwnedOrganization(ctx context.Context, slug, name string, ownerAccountID int64) (database.Organization, error) {
	org, err := app.db.InsertOrg(ctx, slug, name)
	if err != nil {
		return database.Organization{}, err
	}

	err = app.db.AddOrgMember(ctx, org.ID, ownerAccountID)
	if err != nil {
		return database.Organization{}, err
	}

	err = app.enforcer.SeedOrg(ctx, org.ID, ownerAccountID)
	if err != nil {
		return database.Organization{}, err
	}

	return org, nil
}

func (app *application) getOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	err := response.JSON(w, http.StatusOK, org)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

	var input struct {
		Name string              `json:"name"`
		V    validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.V.CheckField(input.Name != "", "name", "Name is required.")

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	err = app.db.UpdateOrg(r.Context(), org.ID, input.Name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	updated, found, err := app.db.GetOrg(r.Context(), org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = response.JSON(w, http.StatusOK, updated)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)

	err := app.db.DeleteOrg(r.Context(), org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidateOrgPolicy(org.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) createOrg(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string              `json:"name"`
		Slug string              `json:"slug"`
		V    validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(input.Slug)

	input.V.CheckField(input.Name != "", "name", "Name is required.")

	slug := input.Slug
	if slug == "" {
		slug = slugify(input.Name)
	}

	input.V.CheckField(slug != "", "slug", "Slug is required.")
	if slug != "" {
		input.V.CheckField(isValidSlug(slug), "slug", "Slug may only contain lowercase letters, numbers, and hyphens.")
	}

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account := contextGetAccount(r)
	org, err := app.createOwnedOrganization(r.Context(), slug, input.Name, account.ID)
	if err != nil {
		if isUniqueViolation(err) {
			if input.Slug != "" {
				app.failedDuplicateField(w, r, "slug", "An organization with this slug already exists.")
				return
			}
			app.failedDuplicateField(w, r, "name", "An organization with this name already exists.")
			return
		}
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, org)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"email":      "email",
		"created_at": "joined_at",
	})
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && role != access.BuiltinOrgOwnerRole && role != access.BuiltinOrgAdminRole && role != access.BuiltinOrgMemberRole {
		errs["role"] = "Role must be Organization Owner, Organization Admin, or Organization Member."
	}
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.db.ListOrgMembersPage(r.Context(), database.ListOrgMembersParams{
		OrgID:    org.ID,
		Search:   q.Search,
		Role:     role,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, members)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMemberCandidates(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"id":         "id",
		"email":      "email",
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) > 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	org := contextGetOrg(r)
	accounts, err := app.db.ListAccountsPage(r.Context(), database.ListAccountsParams{
		ExcludeOrgID: org.ID,
		Search:       q.Search,
		Sort:         q.Sort,
		Order:        q.Order,
		Page:         q.Page,
		PageSize:     q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, accounts)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addOrgMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountID int64               `json:"account_id"`
		Email     string              `json:"email"`
		V         validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.Email = strings.TrimSpace(input.Email)
	input.V.CheckField(input.AccountID > 0 || input.Email != "", "account", "Select a user.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	var (
		account database.Account
		found   bool
	)
	if input.AccountID > 0 {
		account, found, err = app.db.GetAccount(r.Context(), input.AccountID)
	} else {
		account, found, err = app.db.GetAccountByEmail(r.Context(), input.Email)
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.AddOrgMember(r.Context(), org.ID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// No role binding is created here. The new member has no workspace access by default;
	// a workspace admin must explicitly grant access to each workspace.
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getOrgMember(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	member, found, err := app.db.GetOrgMember(r.Context(), org.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	err = response.JSON(w, http.StatusOK, member)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMemberTeams(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	isMember, err := app.db.IsOrgMember(r.Context(), org.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !isMember {
		app.notFound(w, r)
		return
	}

	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	slug := strings.TrimSpace(r.URL.Query().Get("slug"))
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	teams, err := app.db.ListAccountTeamsPage(r.Context(), database.ListTeamsParams{
		OrgID:    org.ID,
		Search:   q.Search,
		Slug:     slug,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	}, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, teams)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountIDStr := chi.URLParam(r, "account_id")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	// Prevent removing the last owner.
	if isLastOwner, checkErr := app.isLastOrgOwner(r, org.ID, accountID); checkErr != nil {
		app.serverError(w, r, checkErr)
		return
	} else if isLastOwner {
		v := validator.Validator{}
		v.AddError("Cannot remove the last owner of an organization.")
		app.failedValidation(w, r, v)
		return
	}

	err = app.db.RemoveOrgMember(r.Context(), org.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, accountID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) updateOrgMemberRole(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Role string              `json:"role"`
		V    validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Role != "", "role", "Role is required.")
	input.V.CheckField(input.Role == access.BuiltinOrgOwnerRole || input.Role == access.BuiltinOrgAdminRole, "role", "Role must be Organization Owner or Organization Admin.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	accountIDStr := chi.URLParam(r, "account_id")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	// Prevent demoting the last owner.
	if input.Role != access.BuiltinOrgOwnerRole {
		if isLastOwner, checkErr := app.isLastOrgOwner(r, org.ID, accountID); checkErr != nil {
			app.serverError(w, r, checkErr)
			return
		} else if isLastOwner {
			v := validator.Validator{}
			v.AddError("Cannot demote the last owner of an organization.")
			app.failedValidation(w, r, v)
			return
		}
	}

	roles, err := app.db.ListOrgRoles(r.Context(), org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	var roleID int64
	for _, role := range roles {
		if role.Name == input.Role && role.IsBuiltin {
			roleID = role.ID
			break
		}
	}
	if roleID == 0 {
		app.notFound(w, r)
		return
	}

	isMember, err := app.db.IsOrgMember(r.Context(), org.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !isMember {
		app.notFound(w, r)
		return
	}

	var builtinRoleIDs []int64
	for _, role := range roles {
		if role.IsBuiltin && (role.Name == access.BuiltinOrgOwnerRole || role.Name == access.BuiltinOrgAdminRole) {
			builtinRoleIDs = append(builtinRoleIDs, role.ID)
		}
	}

	err = app.db.DeleteAccountRoleBindings(r.Context(), org.ID, accountID, "org", org.ID, builtinRoleIDs)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	grantor := contextGetAccount(r)
	err = app.enforcer.BindRole(r.Context(), org.ID, roleID, "account", accountID, "org", org.ID, grantor.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, accountID)
	w.WriteHeader(http.StatusNoContent)
}

// isLastOrgOwner returns true if accountID is the only owner of the org.
func (app *application) isLastOrgOwner(r *http.Request, orgID, accountID int64) (bool, error) {
	roles, err := app.db.ListOrgRoles(r.Context(), orgID)
	if err != nil {
		return false, err
	}
	var ownerRoleID int64
	for _, role := range roles {
		if role.Name == access.BuiltinOrgOwnerRole && role.IsBuiltin {
			ownerRoleID = role.ID
			break
		}
	}
	if ownerRoleID == 0 {
		return false, nil
	}
	n, err := app.db.CountRoleBinding(r.Context(), orgID, ownerRoleID, "org", orgID)
	if err != nil {
		return false, err
	}
	if n <= 1 {
		// Check that the target account actually holds the owner role.
		holds, err := app.db.AccountHasRoleBinding(r.Context(), orgID, ownerRoleID, accountID, "org", orgID)
		if err != nil {
			return false, err
		}
		return holds, nil
	}
	return false, nil
}

// slugify converts a name to a URL-safe slug.
func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, s)
	s = strings.Trim(s, "-")
	if len(s) > 50 {
		s = s[:50]
	}
	return s
}

// isValidSlug returns true if s contains only lowercase letters, digits, and hyphens.
func isValidSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}
