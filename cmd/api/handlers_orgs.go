package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) getOrg(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	err := response.JSON(w, http.StatusOK, org)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createOrg(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string              `json:"name"`
		V    validator.Validator `json:"-"`
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

	slug := slugify(input.Name)
	org, err := app.db.InsertOrg(r.Context(), slug, input.Name)
	if err != nil {
		if isUniqueViolation(err) {
			input.V.AddFieldError("name", "an organization with this name already exists")
			app.failedValidation(w, r, input.V)
			return
		}
		app.serverError(w, r, err)
		return
	}

	account := contextGetAccount(r)
	err = app.db.AddOrgMember(r.Context(), org.ID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.SeedOrg(r.Context(), org.ID, account.ID)
	if err != nil {
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
	members, err := app.db.GetOrgMembers(r.Context(), org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, members)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addOrgMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string              `json:"email"`
		V     validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Email != "", "email", "email is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	account, found, err := app.db.GetAccountByEmail(r.Context(), input.Email)
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
		v.AddError("cannot remove the last owner of an organization")
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

	input.V.CheckField(input.Role != "", "role", "role is required")
	input.V.CheckField(input.Role == "owner" || input.Role == "admin", "role", "role must be owner or admin")
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
	if input.Role != "owner" {
		if isLastOwner, checkErr := app.isLastOrgOwner(r, org.ID, accountID); checkErr != nil {
			app.serverError(w, r, checkErr)
			return
		} else if isLastOwner {
			v := validator.Validator{}
			v.AddError("cannot demote the last owner of an organization")
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
		if role.Name == "owner" && role.IsBuiltin {
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
