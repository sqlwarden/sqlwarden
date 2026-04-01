package main

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listTeams(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	teams, err := app.db.ListTeams(org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, teams)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createTeam(w http.ResponseWriter, r *http.Request) {
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

	org := contextGetOrg(r)
	slug := slugify(input.Name)
	team, err := app.db.InsertTeam(org.ID, slug, input.Name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusCreated, team)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getTeam(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	err = response.JSON(w, http.StatusOK, team)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteTeam(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	err = app.db.DeleteTeam(team.ID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	members, err := app.db.ListTeamMembers(team.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, members)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addTeamMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountID int64               `json:"account_id"`
		V         validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.AccountID > 0, "account_id", "account_id is required")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.AddTeamMember(team.ID, input.AccountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, input.AccountID)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	accountIDStr := chi.URLParam(r, "account_id")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	err = app.db.RemoveTeamMember(team.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, accountID)
	w.WriteHeader(http.StatusNoContent)
}
