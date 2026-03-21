package main

import (
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

var rgxSlugValid = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func (app *application) listTeams(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	teams, err := app.db.GetTeamsByTenant(tenant.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if teams == nil {
		teams = []database.Team{}
	}

	err = response.JSON(w, http.StatusOK, teams)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createTeam(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)

	var input struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator

	v.CheckField(validator.NotBlank(input.Slug), "slug", "Slug is required")
	v.CheckField(validator.MaxRunes(input.Slug, 50), "slug", "Must not be more than 50 characters")
	if validator.NotBlank(input.Slug) {
		v.CheckField(rgxSlugValid.MatchString(input.Slug), "slug", "Must be lowercase alphanumeric with hyphens only")
	}
	v.CheckField(validator.NotBlank(input.Name), "name", "Name is required")
	v.CheckField(validator.MaxRunes(input.Name, 100), "name", "Must not be more than 100 characters")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	team, err := app.db.InsertTeam(tenant.ID, input.Slug, input.Name)
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
	tenant, _ := contextGetTenant(r)
	teamSlug := chi.URLParam(r, "team_slug")

	team, found, err := app.db.GetTeamBySlug(tenant.ID, teamSlug)
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
	tenant, _ := contextGetTenant(r)
	teamSlug := chi.URLParam(r, "team_slug")

	team, found, err := app.db.GetTeamBySlug(tenant.ID, teamSlug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.DeleteTeam(team.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)
	teamSlug := chi.URLParam(r, "team_slug")

	team, found, err := app.db.GetTeamBySlug(tenant.ID, teamSlug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	members, err := app.db.GetTeamMembers(team.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if members == nil {
		members = []database.TeamMember{}
	}

	err = response.JSON(w, http.StatusOK, members)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addTeamMember(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)
	teamSlug := chi.URLParam(r, "team_slug")

	team, found, err := app.db.GetTeamBySlug(tenant.ID, teamSlug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	var input struct {
		AccountID string `json:"account_id"`
	}

	err = request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	var v validator.Validator
	v.CheckField(validator.NotBlank(input.AccountID), "account_id", "Account ID is required")

	if v.HasErrors() {
		app.failedValidation(w, r, v)
		return
	}

	err = app.db.AddTeamMember(team.ID, input.AccountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.AddTeamMember(input.AccountID, team.ID, tenant.Slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	tenant, _ := contextGetTenant(r)
	teamSlug := chi.URLParam(r, "team_slug")
	accountID := chi.URLParam(r, "account_id")

	team, found, err := app.db.GetTeamBySlug(tenant.ID, teamSlug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.RemoveTeamMember(team.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.enforcer.RemoveTeamMember(accountID, team.ID, tenant.Slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
