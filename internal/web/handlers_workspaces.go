package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listWorkspaces(w http.ResponseWriter, r *http.Request) {
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
	result, err := app.services.Catalog.ListWorkspaces(r.Context(), principal, ids.OrgID, catalog.ListQuery{
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
	ws, err := app.services.Catalog.CreateWorkspace(r.Context(), principal, ids.OrgID, catalog.CreateWorkspaceInput{Name: input.Name, Description: input.Description})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace created", slog.Int64("org_id", ids.OrgID), slog.Int64("workspace_id", ws.ID), slog.Int64("owner_account_id", principal.Subject.ID))
	if err := response.JSON(w, http.StatusCreated, ws); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	ws, err := app.services.Catalog.GetWorkspace(r.Context(), principal, ids.OrgID, ids.WorkspaceID)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, ws); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		OrgID       *int64  `json:"org_id"`
		OwnerType   *string `json:"owner_type"`
		OwnerID     *int64  `json:"owner_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	ids := contextGetPathIDs(r)
	err := app.services.Catalog.UpdateWorkspace(r.Context(), principal, ids.OrgID, ids.WorkspaceID, catalog.UpdateWorkspaceInput{
		Name:        input.Name,
		Description: input.Description,
		OrgID:       input.OrgID,
		OwnerType:   input.OwnerType,
		OwnerID:     input.OwnerID,
	})
	if err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace updated", slog.Int64("workspace_id", ids.WorkspaceID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) deleteWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ids := contextGetPathIDs(r)
	if err := app.services.Catalog.DeleteWorkspace(r.Context(), principal, ids.OrgID, ids.WorkspaceID); err != nil {
		app.catalogError(w, r, err)
		return
	}

	app.logInfo(r, "workspace deleted", slog.Int64("workspace_id", ids.WorkspaceID))
	w.WriteHeader(http.StatusNoContent)
}
