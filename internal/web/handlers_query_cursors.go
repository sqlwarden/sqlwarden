package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/execution"
	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/pkg/result"
)

const apiErrorQueryCursorUnavailable = "query_cursor_unavailable"

type queryCursorRequest struct {
	SQL      string `json:"sql"`
	PageSize *int   `json:"page_size"`
}

type queryCursorFetchRequest struct {
	PageSize *int `json:"page_size"`
}

type queryCursorPageResponse struct {
	QueryCursorID    string          `json:"query_cursor_id"`
	Columns          []result.Column `json:"columns"`
	Rows             []result.Row    `json:"rows"`
	DurationMs       int64           `json:"duration_ms"`
	Truncated        bool            `json:"truncated"`
	RowsReturned     int             `json:"rows_returned"`
	BytesReturned    int64           `json:"bytes_returned"`
	TruncationReason string          `json:"truncation_reason,omitempty"`
	Exhausted        bool            `json:"exhausted"`
	PageSize         int             `json:"page_size"`
}

func queryLimits(settings effectiveRuntimeSettings) execution.Limits {
	return execution.Limits{MaxRows: settings.QueryMaxResultRows, MaxBytes: settings.QueryMaxResultBytes}
}

func (app *application) startQueryCursor(w http.ResponseWriter, r *http.Request) {
	var input queryCursorRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	sessionID := requestSessionID(r)
	page, err := app.queryService().StartCursor(r.Context(), principal, connectionRef(r), queryapp.CursorInput{
		SessionID: sessionID,
		SQL:       input.SQL,
		PageSize:  input.PageSize,
	})
	if err != nil {
		if errors.Is(err, queryapp.ErrCursorsUnsupported) {
			app.logWarn(r, "query cursor unsupported",
				slog.String("session_id", string(sessionID)),
				slog.Int("page_size", page.PageSize),
			)
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query cursors.", nil)
			return
		}
		var runtimeErr *queryapp.RuntimeError
		if errors.As(err, &runtimeErr) && !isRequestCanceled(r, err) {
			app.logWarn(r, "query cursor start failed",
				slog.String("session_id", string(sessionID)),
				slog.String("error", err.Error()),
			)
		}
		app.queryRuntimeError(w, r, err)
		return
	}

	app.logDebug(r, "query cursor started",
		slog.String("session_id", string(sessionID)),
		slog.String("query_cursor_id", string(page.Result.CursorID)),
		slog.Int("page_size", page.PageSize),
		slog.Bool("exhausted", page.Result.Exhausted),
	)
	app.writeQueryCursorPage(w, r, page.Result, page.PageSize)
}

func (app *application) fetchQueryCursor(w http.ResponseWriter, r *http.Request) {
	var input queryCursorFetchRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := request.DecodeJSON(w, r, &input); err != nil {
			app.badRequest(w, r, err)
			return
		}
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	cursorID := execution.CursorID(strings.TrimSpace(chi.URLParam(r, "query_cursor_id")))
	page, err := app.queryService().FetchCursor(r.Context(), principal, connectionRef(r), queryapp.FetchInput{
		SessionID: requestSessionID(r),
		CursorID:  cursorID,
		PageSize:  input.PageSize,
	})
	if err != nil {
		var runtimeErr *queryapp.RuntimeError
		var failure *execution.Failure
		if errors.As(err, &runtimeErr) && !errors.As(err, &failure) && !isRequestCanceled(r, err) {
			app.logWarn(r, "query cursor fetch failed",
				slog.String("query_cursor_id", string(cursorID)),
				slog.String("error", err.Error()),
			)
		}
		app.queryRuntimeError(w, r, err)
		return
	}

	app.logDebug(r, "query cursor fetched",
		slog.String("query_cursor_id", string(cursorID)),
		slog.Int("page_size", page.PageSize),
		slog.Bool("exhausted", page.Result.Exhausted),
	)
	app.writeQueryCursorPage(w, r, page.Result, page.PageSize)
}

// closeQueryCursor is idempotent: closing an unknown or foreign cursor
// answers 204 exactly like closing one's own.
func (app *application) closeQueryCursor(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	cursorID := execution.CursorID(strings.TrimSpace(chi.URLParam(r, "query_cursor_id")))
	if err := app.queryService().CloseCursor(r.Context(), principal, connectionRef(r), requestSessionID(r), cursorID); err != nil {
		app.queryRuntimeError(w, r, err)
		return
	}
	app.logDebug(r, "query cursor closed", slog.String("query_cursor_id", string(cursorID)))
	w.WriteHeader(http.StatusNoContent)
}

// writeQueryCursorPage answers with one page. A query whose first page
// exhausts its result never leaves a cursor open, so it reports no cursor id.
func (app *application) writeQueryCursorPage(w http.ResponseWriter, r *http.Request, out execution.QueryResult, pageSize int) {
	rs := out.Result
	if rs == nil {
		rs = &result.ResultSet{}
	}
	payload := queryCursorPageResponse{
		QueryCursorID:    string(out.CursorID),
		Columns:          rs.Columns,
		Rows:             rs.Rows,
		DurationMs:       rs.DurationMs,
		Truncated:        rs.Truncated,
		RowsReturned:     rs.RowsReturned,
		BytesReturned:    rs.BytesReturned,
		TruncationReason: rs.TruncationReason,
		Exhausted:        out.Exhausted,
		PageSize:         pageSize,
	}
	if err := response.JSON(w, http.StatusOK, payload); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) queryCursorUnavailable(w http.ResponseWriter, r *http.Request) {
	app.apiError(w, r, http.StatusGone, apiErrorQueryCursorUnavailable, "Query cursor is no longer available. Run the query again.", response.APIError{}, nil)
}
