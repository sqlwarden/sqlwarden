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

func (app *application) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	org := contextGetOrg(r)
	account := contextGetAccount(r)
	result, err := app.catalogService().ListWorkspaces(r.Context(), account.ID, org.ID, catalog.ListQuery{
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

func (app *application) createWorkspace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	org := contextGetOrg(r)
	actor := catalogActor(r)
	ws, err := app.catalogService().CreateWorkspace(r.Context(), actor, org.ID, catalog.CreateWorkspaceInput{
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		if errors.Is(err, catalog.ErrNameTaken) {
			app.failedDuplicateField(w, r, "name", "A workspace with this name already exists in this organization.")
			return
		}
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace created", slog.Int64("org_id", org.ID), slog.Int64("workspace_id", ws.ID), slog.Int64("owner_account_id", actor.AccountID))
	if err := response.JSON(w, http.StatusCreated, ws); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspace(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	account := contextGetAccount(r)
	ws, err := app.catalogService().Workspace(r.Context(), account.ID, org.ID, contextGetWorkspace(r))
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusOK, ws); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string              `json:"name"`
		Description string              `json:"description"`
		OrgID       *int64              `json:"org_id"`
		OwnerType   *string             `json:"owner_type"`
		OwnerID     *int64              `json:"owner_id"`
		V           validator.Validator `json:"-"`
	}

	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.OrgID == nil, "org_id", "Organization is immutable.")
	input.V.CheckField(input.OwnerType == nil, "owner_type", "Owner type is immutable.")
	input.V.CheckField(input.OwnerID == nil, "owner_id", "Owner is immutable.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	ws := contextGetWorkspace(r)
	err := app.catalogService().UpdateWorkspace(r.Context(), catalogActor(r), ws, catalog.CreateWorkspaceInput{
		Name:        input.Name,
		Description: input.Description,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace updated", slog.Int64("workspace_id", ws.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	if err := app.catalogService().DeleteWorkspace(r.Context(), catalogActor(r), ws); err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace deleted", slog.Int64("workspace_id", ws.ID))
	w.WriteHeader(http.StatusNoContent)
}
