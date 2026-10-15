package web

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listQueryFavorites(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	favorites, err := app.queryService().ListFavorites(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, r.URL.Query().Get("q"))
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusOK, map[string]any{"items": favorites}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createQueryFavorite(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string `json:"name"`
		SQL          string `json:"sql_text"`
		ConnectionID *int64 `json:"connection_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	favorite, err := app.queryService().CreateFavorite(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, query.FavoriteInput{
		Name:         input.Name,
		SQL:          input.SQL,
		ConnectionID: input.ConnectionID,
	})
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusCreated, favorite); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateQueryFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "favorite_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	var input struct {
		Name string `json:"name"`
		SQL  string `json:"sql_text"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	updated, err := app.queryService().UpdateFavorite(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, id, query.FavoriteInput{Name: input.Name, SQL: input.SQL})
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusOK, updated); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteQueryFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "favorite_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	if err := app.queryService().DeleteFavorite(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, id); err != nil {
		app.queryError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
