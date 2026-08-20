package web

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listQueryFavorites(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)

	search := r.URL.Query().Get("q")

	favorites, err := app.db.ListQueryFavorites(r.Context(), ws.ID, account.ID, search)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusOK, map[string]any{"items": favorites}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createQueryFavorite(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)

	var input struct {
		Name         string              `json:"name"`
		SQL          string              `json:"sql_text"`
		ConnectionID *int64              `json:"connection_id"`
		V            validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.SQL != "", "sql_text", "SQL text is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	effective, err := app.runtimeSettingsService().effectiveForOrg(r.Context(), ws.OrgID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	input.V.Check(effective.QueryFavoritesMode == "backend", "Query favorites are not enabled for this workspace.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	favorite, err := app.db.CreateQueryFavorite(r.Context(), database.QueryFavorite{
		WorkspaceID:  ws.ID,
		AccountID:    account.ID,
		ConnectionID: input.ConnectionID,
		Name:         input.Name,
		SQL:          input.SQL,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusCreated, favorite); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateQueryFavorite(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "favorite_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	var input struct {
		Name string              `json:"name"`
		SQL  string              `json:"sql_text"`
		V    validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(input.SQL != "", "sql_text", "SQL text is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	updated, found, err := app.db.UpdateQueryFavorite(r.Context(), id, ws.ID, account.ID, input.Name, input.SQL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	if err := response.JSON(w, http.StatusOK, updated); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteQueryFavorite(w http.ResponseWriter, r *http.Request) {
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "favorite_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}

	found, err := app.db.DeleteQueryFavorite(r.Context(), id, ws.ID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
