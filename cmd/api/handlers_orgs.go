package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) getOrg(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	err := response.JSON(w, http.StatusOK, tenant)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	members, err := app.db.GetTenantMembers(tenant.ID)
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
	tenant, _ := contextGetTenant(r)

	var input struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator

	v.CheckField(validator.NotBlank(input.Email), "email", "Email is required")
	v.CheckField(validator.In(input.Role, "admin", "member"), "role", "Role must be admin or member")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	account, found, err := app.db.GetAccountByEmail(input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.AddTenantMember(tenant.ID, account.ID, input.Role)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.SetOrgRole(account.ID, input.Role, tenant.Slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) updateOrgMemberRole(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	var input struct {
		Role string `json:"role"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator

	v.CheckField(validator.In(input.Role, "owner", "admin", "member"), "role", "Role must be owner, admin, or member")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	accountID := chi.URLParam(r, "account_id")

	if input.Role != "owner" {
		count, err := app.db.CountTenantOwners(tenant.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}

		if count == 1 {
			// Check if the target account is currently an owner.
			members, err := app.db.GetTenantMembers(tenant.ID)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
			for _, m := range members {
				if m.AccountID == accountID && m.Role == "owner" {
					err = response.JSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Cannot demote the last owner"})
					if err != nil {
						app.serverError(w, r, err)
					}
					return
				}
			}
		}
	}

	err = app.db.UpdateTenantMemberRole(tenant.ID, accountID, input.Role)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.SetOrgRole(accountID, input.Role, tenant.Slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)
	accountID := chi.URLParam(r, "account_id")

	members, err := app.db.GetTenantMembers(tenant.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	var found bool
	var memberRole string
	for _, m := range members {
		if m.AccountID == accountID {
			found = true
			memberRole = m.Role
			break
		}
	}

	if !found {
		app.notFound(w, r)
		return
	}

	if memberRole == "owner" {
		count, err := app.db.CountTenantOwners(tenant.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if count == 1 {
			err = response.JSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Cannot remove the last owner"})
			if err != nil {
				app.serverError(w, r, err)
			}
			return
		}
	}

	err = app.db.RemoveTenantMember(tenant.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.RemoveOrgMember(accountID, tenant.Slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
