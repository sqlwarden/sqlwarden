package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"email":      "email",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.db.ListWorkspaceMembersPage(r.Context(), database.ListWorkspaceMembersParams{
		WorkspaceID: ws.ID,
		Search:      q.Search,
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err = response.JSON(w, http.StatusOK, members); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listWorkspaceEffectiveMembers(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"email":      "email",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.db.ListWorkspaceEffectiveMembersPage(r.Context(), org.ID, database.ListWorkspaceMembersParams{
		WorkspaceID: ws.ID,
		Search:      q.Search,
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err = response.JSON(w, http.StatusOK, members); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountID int64               `json:"account_id"`
		V         validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.AccountID > 0, "account_id", "Account is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	actor := contextGetAccount(r)

	isMember, err := app.db.IsOrgMember(r.Context(), org.ID, input.AccountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !isMember {
		app.notFound(w, r)
		return
	}

	if err = app.db.AddWorkspaceMember(r.Context(), ws.ID, input.AccountID, &actor.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, input.AccountID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	if err = app.db.RemoveWorkspaceMember(r.Context(), ws.ID, accountID); err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, accountID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listWorkspaceTeams(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"slug":       "slug",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	teams, err := app.db.ListWorkspaceTeamsPage(r.Context(), database.ListWorkspaceTeamsParams{
		WorkspaceID: ws.ID,
		Search:      q.Search,
		Sort:        q.Sort,
		Order:       q.Order,
		Page:        q.Page,
		PageSize:    q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err = response.JSON(w, http.StatusOK, teams); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addWorkspaceTeam(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TeamID int64               `json:"team_id"`
		V      validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.TeamID > 0, "team_id", "Team is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	actor := contextGetAccount(r)

	team, found, err := app.db.GetTeamByID(r.Context(), input.TeamID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || team.OrgID != org.ID {
		app.notFound(w, r)
		return
	}

	if err = app.db.AddWorkspaceTeam(r.Context(), ws.ID, input.TeamID, &actor.ID); err != nil {
		app.serverError(w, r, err)
		return
	}

	accountIDs, err := app.db.ListWorkspaceTeamMemberAccountIDs(r.Context(), ws.ID, input.TeamID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	for _, accountID := range accountIDs {
		app.enforcer.InvalidatePrincipals(org.ID, accountID)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeWorkspaceTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	accountIDs, err := app.db.ListWorkspaceTeamMemberAccountIDs(r.Context(), ws.ID, teamID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err = app.db.RemoveWorkspaceTeam(r.Context(), ws.ID, teamID); err != nil {
		app.serverError(w, r, err)
		return
	}
	for _, accountID := range accountIDs {
		app.enforcer.InvalidatePrincipals(org.ID, accountID)
	}
	w.WriteHeader(http.StatusNoContent)
}
