package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listEnvironments(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	account := contextGetAccount(r)
	result, err := app.catalogService().ListEnvironments(r.Context(), account.ID, org.ID, ws.ID, catalog.ListQuery{
		Search: q.Search, Name: name, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, result)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createEnvironment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	ws := contextGetWorkspace(r)
	env, err := app.catalogService().CreateEnvironment(r.Context(), catalogActor(r), ws.ID, catalog.CreateEnvironmentInput{
		Name: input.Name, Description: input.Description,
	})
	if err != nil {
		if errors.Is(err, catalog.ErrNameTaken) {
			app.failedDuplicateField(w, r, "name", "An environment with this name already exists in this workspace.")
			return
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "environment created", slog.Int64("workspace_id", ws.ID), slog.Int64("environment_id", env.ID))
	err = response.JSON(w, http.StatusCreated, env)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEnvironment(w http.ResponseWriter, r *http.Request) {
	env := contextGetEnvironment(r)
	ws := contextGetWorkspace(r)
	resolved, err := app.catalogService().Environment(r.Context(), contextGetAccount(r).ID, contextGetOrg(r).ID, ws, env)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, resolved)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		WorkspaceID *int64              `json:"workspace_id"`
		V           validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.WorkspaceID == nil, "workspace_id", "Workspace is immutable.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	env := contextGetEnvironment(r)
	err = app.catalogService().UpdateEnvironment(r.Context(), catalogActor(r), env, catalog.CreateEnvironmentInput{
		Name: input.Name, Description: input.Description,
	})
	if err != nil {
		if errors.Is(err, catalog.ErrNameTaken) {
			app.failedDuplicateField(w, r, "name", "An environment with this name already exists in this workspace.")
			return
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "environment updated", slog.Int64("environment_id", env.ID), slog.Int64("workspace_id", env.WorkspaceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	env := contextGetEnvironment(r)

	err := app.catalogService().DeleteEnvironment(r.Context(), catalogActor(r), env)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "environment deleted", slog.Int64("environment_id", env.ID), slog.Int64("workspace_id", env.WorkspaceID))
	w.WriteHeader(http.StatusNoContent)
}
