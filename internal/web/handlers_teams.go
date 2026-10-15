package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/orgs"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listTeams(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	teams, err := app.services.Orgs.ListTeams(r.Context(), principal, contextGetPathIDs(r).OrgID, orgs.ListTeamsParams{
		Search:   q.Search,
		Slug:     strings.TrimSpace(r.URL.Query().Get("slug")),
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, teams); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createTeam(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
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
	team, err := app.services.Orgs.CreateTeam(r.Context(), principal, contextGetPathIDs(r).OrgID, orgs.CreateTeamInput{Slug: input.Slug, Name: input.Name})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "team created", slog.Int64("org_id", ids.OrgID), slog.Int64("team_id", team.ID), slog.String("team_slug", team.Slug))
	if err := response.JSON(w, http.StatusCreated, team); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	team, err := app.services.Orgs.GetTeam(r.Context(), principal, contextGetPathIDs(r).OrgID, chi.URLParam(r, "team_slug"))
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, team); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateTeam(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string  `json:"name"`
		Slug  *string `json:"slug"`
		OrgID *int64  `json:"org_id"`
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
	team, err := app.services.Orgs.UpdateTeam(r.Context(), principal, contextGetPathIDs(r).OrgID, chi.URLParam(r, "team_slug"), orgs.UpdateTeamInput{
		Name:  input.Name,
		Slug:  input.Slug,
		OrgID: input.OrgID,
	})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "team updated", slog.Int64("org_id", ids.OrgID), slog.Int64("team_id", team.ID), slog.String("team_slug", team.Slug))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	team, err := app.services.Orgs.DeleteTeam(r.Context(), principal, contextGetPathIDs(r).OrgID, chi.URLParam(r, "team_slug"))
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "team deleted", slog.Int64("org_id", ids.OrgID), slog.Int64("team_id", team.ID), slog.String("team_slug", team.Slug))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listTeamMembers(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	orgID := contextGetPathIDs(r).OrgID
	slug := chi.URLParam(r, "team_slug")

	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"account_id": "account_id",
		"created_at": "created_at",
	})
	if _, ok := r.URL.Query()["sort"]; !ok {
		q.Sort = "created_at"
	}
	if _, ok := r.URL.Query()["order"]; !ok {
		q.Order = "asc"
	}
	if len(errs) != 0 {
		// An unknown team answers 404 before the query is validated.
		if _, err := app.services.Orgs.GetTeam(r.Context(), principal, orgID, slug); err != nil {
			app.orgError(w, r, err, "")
			return
		}
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.services.Orgs.ListTeamMembers(r.Context(), principal, orgID, slug, orgs.ListTeamMembersParams{
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	if err := response.JSON(w, http.StatusOK, members); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addTeamMember(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountID int64 `json:"account_id"`
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
	team, err := app.services.Orgs.AddTeamMember(r.Context(), principal, contextGetPathIDs(r).OrgID, chi.URLParam(r, "team_slug"), input.AccountID)
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "team member added", slog.Int64("org_id", ids.OrgID), slog.Int64("team_id", team.ID), slog.Int64("target_account_id", input.AccountID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	slug := chi.URLParam(r, "team_slug")
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	team, affected, err := app.services.Orgs.RemoveTeamMember(r.Context(), principal, contextGetPathIDs(r).OrgID, slug, accountID)
	if err != nil {
		app.orgError(w, r, err, "")
		return
	}
	app.logInfo(r, "team member removed", slog.Int64("org_id", ids.OrgID), slog.Int64("team_id", team.ID), slog.Int64("target_account_id", accountID), slog.Int("affected_workspaces", affected))
	w.WriteHeader(http.StatusNoContent)
}
