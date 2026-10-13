package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listEnvironments(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	ids := contextGetPathIDs(r)
	result, err := app.services.Catalog.ListEnvironments(r.Context(), principal, ids.OrgID, ids.WorkspaceID, catalog.ListQuery{
		Search:   q.Search,
		Name:     strings.TrimSpace(r.URL.Query().Get("name")),
		Sort:     q.Sort,
		Order:    q.Order,
		Page:     q.Page,
		PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createEnvironment(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	ids := contextGetPathIDs(r)
	env, err := app.services.Catalog.CreateEnvironment(r.Context(), principal, ids.OrgID, ids.WorkspaceID, catalog.EnvironmentInput{Name: input.Name, Description: input.Description})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "environment created", slog.Int64("workspace_id", ids.WorkspaceID), slog.Int64("environment_id", env.ID))
	if err := response.JSON(w, http.StatusCreated, env); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEnvironment(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	env, err := app.services.Catalog.GetEnvironment(r.Context(), principal, ids.OrgID, ids.WorkspaceID, ids.EnvironmentID)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, env); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		WorkspaceID *int64 `json:"workspace_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	ids := contextGetPathIDs(r)
	err := app.services.Catalog.UpdateEnvironment(r.Context(), principal, ids.OrgID, ids.WorkspaceID, ids.EnvironmentID, catalog.UpdateEnvironmentInput{
		Name:        input.Name,
		Description: input.Description,
		WorkspaceID: input.WorkspaceID,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "environment updated", slog.Int64("environment_id", ids.EnvironmentID), slog.Int64("workspace_id", ids.WorkspaceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	affected, err := app.services.Catalog.DeleteEnvironment(r.Context(), principal, ids.OrgID, ids.WorkspaceID, ids.EnvironmentID)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "environment deleted", slog.Int64("environment_id", ids.EnvironmentID), slog.Int64("workspace_id", ids.WorkspaceID), slog.Int("affected_connections", affected))
	w.WriteHeader(http.StatusNoContent)
}
