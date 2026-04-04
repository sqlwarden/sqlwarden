package main

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listTeams(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	teams, err := app.db.ListTeamsFiltered(context.Background(), database.ListTeamsParams{
		OrgID:  org.ID,
		Search: q.Search,
		Sort:   q.Sort,
		Order:  q.Order,
	})
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
		Slug string              `json:"slug"`
		Name string              `json:"name"`
		V    validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.Slug != "", "slug", "slug is required")
	if input.Slug != "" {
		input.V.CheckField(isValidSlug(input.Slug), "slug", "slug may only contain lowercase letters, numbers, and hyphens")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	slug := input.Slug
	team, err := app.db.InsertTeam(context.Background(), org.ID, slug, input.Name)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "slug", "a team with this slug already exists in this organization")
			return
		}
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
	team, found, err := app.db.GetTeam(context.Background(), org.ID, slug)
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

func (app *application) updateTeam(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string              `json:"name"`
		Slug  *string             `json:"slug"`
		OrgID *int64              `json:"org_id"`
		V     validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "name is required")
	input.V.CheckField(input.Slug == nil, "slug", "is immutable")
	input.V.CheckField(input.OrgID == nil, "org_id", "is immutable")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(context.Background(), org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	if err := app.db.UpdateTeam(context.Background(), team.ID, org.ID, input.Name); err != nil {
		app.serverError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteTeam(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(context.Background(), org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	err = app.db.DeleteTeam(context.Background(), team.ID, org.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	slug := chi.URLParam(r, "team_slug")
	team, found, err := app.db.GetTeam(context.Background(), org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	members, err := app.db.ListTeamMembers(context.Background(), team.ID)
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
	team, found, err := app.db.GetTeam(context.Background(), org.ID, slug)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	err = app.db.AddTeamMember(context.Background(), team.ID, input.AccountID)
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
	team, found, err := app.db.GetTeam(context.Background(), org.ID, slug)
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

	err = app.db.RemoveTeamMember(context.Background(), team.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.enforcer.InvalidatePrincipals(org.ID, accountID)
	w.WriteHeader(http.StatusNoContent)
}
