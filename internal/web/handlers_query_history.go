package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

func (app *application) createQueryHistoryEntry(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SQL          string  `json:"sql_text"`
		Status       string  `json:"status"`
		ErrorMessage *string `json:"error_message"`
		DurationMS   int64   `json:"duration_ms"`
		RowsAffected int64   `json:"rows_affected"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	saved, err := app.queryService().RecordHistory(r.Context(), principal, connectionRef(r), query.HistoryInput{
		SQL:          input.SQL,
		Status:       input.Status,
		ErrorMessage: input.ErrorMessage,
		DurationMS:   input.DurationMS,
		RowsAffected: input.RowsAffected,
	})
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusCreated, saved); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listQueryHistory(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), nil)
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	result, err := app.queryService().ListHistory(r.Context(), principal, connectionRef(r), query.ListQuery{Search: q.Search, Page: q.Page, PageSize: q.PageSize})
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listWorkspaceQueryHistory(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), nil)
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	var connectionID *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("connection_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			app.failedValidation(w, r, fieldErrors(map[string]string{"connection_id": "Connection ID must be an integer."}))
			return
		}
		connectionID = &id
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	result, err := app.queryService().ListWorkspaceHistory(r.Context(), principal, contextGetOrg(r).ID, contextGetWorkspace(r).ID, connectionID, query.ListQuery{Search: q.Search, Page: q.Page, PageSize: q.PageSize})
	if err != nil {
		app.queryError(w, r, err)
		return
	}

	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deleteQueryHistoryEntry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "history_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	if err := app.queryService().DeleteHistoryEntry(r.Context(), principal, connectionRef(r), id); err != nil {
		app.queryError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (app *application) clearQueryHistoryForConnection(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	if err := app.queryService().ClearHistory(r.Context(), principal, connectionRef(r)); err != nil {
		app.queryError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
