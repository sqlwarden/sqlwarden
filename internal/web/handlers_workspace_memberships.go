package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

var workspaceMemberSorts = map[string]string{
	"name":       "name",
	"email":      "email",
	"created_at": "created_at",
}

func (app *application) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	q, errs := readListQuery(r.URL.Query(), workspaceMemberSorts)
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.services.Catalog.ListWorkspaceMembers(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, catalog.ListWorkspaceMembersParams{
		Search:   q.Search,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err = response.JSON(w, http.StatusOK, members); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listWorkspaceEffectiveMembers(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	q, errs := readListQuery(r.URL.Query(), workspaceMemberSorts)
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	members, err := app.services.Catalog.ListWorkspaceEffectiveMembers(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, catalog.ListWorkspaceMembersParams{
		Search:   q.Search,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err = response.JSON(w, http.StatusOK, members); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		AccountID int64 `json:"account_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	ids := contextGetPathIDs(r)
	if err := app.services.Catalog.AddWorkspaceMember(r.Context(), principal, ids.OrgID, ids.WorkspaceID, input.AccountID); err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace member added", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("target_account_id", input.AccountID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	ids := contextGetPathIDs(r)
	if err := app.services.Catalog.RemoveWorkspaceMember(r.Context(), principal, ids.OrgID, ids.WorkspaceID, accountID); err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace member removed", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("target_account_id", accountID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listWorkspaceTeams(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"slug":       "slug",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	teams, err := app.services.Catalog.ListWorkspaceTeams(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, catalog.ListWorkspaceTeamsParams{
		Search:   q.Search,
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err = response.JSON(w, http.StatusOK, teams); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) addWorkspaceTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		TeamID int64 `json:"team_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	ids := contextGetPathIDs(r)
	affected, err := app.services.Catalog.AddWorkspaceTeam(r.Context(), principal, ids.OrgID, ids.WorkspaceID, input.TeamID)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace team added", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("team_id", input.TeamID), slog.Int("affected_accounts", affected))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) removeWorkspaceTeam(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	teamID, err := strconv.ParseInt(chi.URLParam(r, "team_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	ids := contextGetPathIDs(r)
	affected, err := app.services.Catalog.RemoveWorkspaceTeam(r.Context(), principal, ids.OrgID, ids.WorkspaceID, teamID)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace team removed", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("team_id", teamID), slog.Int("affected_accounts", affected))
	w.WriteHeader(http.StatusNoContent)
}
