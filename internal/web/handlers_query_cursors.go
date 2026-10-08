package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"github.com/sqlwarden/pkg/result"
)

const apiErrorQueryCursorUnavailable = "query_cursor_unavailable"

type queryCursorRequest struct {
	SQL      string              `json:"sql"`
	PageSize *int                `json:"page_size"`
	V        validator.Validator `json:"-"`
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

	input.V.CheckField(strings.TrimSpace(input.SQL) != "", "sql", "SQL is required.")
	if input.PageSize != nil {
		input.V.CheckField(*input.PageSize > 0, "page_size", "Page size must be greater than 0.")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	runtimeSettings, err := app.effectiveRuntimeSettingsForWorkspace(r.Context(), contextGetWorkspace(r))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pageSize := queryCursorPageSize(input.PageSize, runtimeSettings)

	_, allowed, err := app.requiredConnectionRuntimePermission(r, input.SQL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !allowed {
		app.notPermitted(w, r)
		return
	}
	sessionID, ok := app.requireSessionID(w, r)
	if !ok {
		return
	}

	out, err := app.runtime.Query(r.Context(), runtimeScope(r), execution.QueryRequest{
		SessionID:     sessionID,
		SQL:           input.SQL,
		Limits:        queryLimits(runtimeSettings),
		UseCursor:     true,
		RequireCursor: true,
		PageSize:      pageSize,
	})
	if err != nil {
		if errors.Is(err, execution.ErrCursorsUnsupported) {
			app.logWarn(r, "query cursor unsupported",
				slog.String("session_id", string(sessionID)),
				slog.Int("page_size", pageSize),
			)
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query cursors.", nil)
			return
		}
		if !isRequestCanceled(r, err) {
			app.logWarn(r, "query cursor start failed",
				slog.String("session_id", string(sessionID)),
				slog.String("error", err.Error()),
			)
		}
		app.executionError(w, r, err)
		return
	}

	app.logDebug(r, "query cursor started",
		slog.String("session_id", string(sessionID)),
		slog.String("query_cursor_id", string(out.CursorID)),
		slog.Int("page_size", pageSize),
		slog.Bool("exhausted", out.Exhausted),
	)
	app.writeQueryCursorPage(w, r, out, pageSize)
}

func (app *application) fetchQueryCursor(w http.ResponseWriter, r *http.Request) {
	var input queryCursorFetchRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := request.DecodeJSON(w, r, &input); err != nil {
			app.badRequest(w, r, err)
			return
		}
	}
	if input.PageSize != nil && *input.PageSize <= 0 {
		app.failedValidation(w, r, fieldErrors(map[string]string{"page_size": "Page size must be greater than 0."}))
		return
	}

	runtimeSettings, err := app.effectiveRuntimeSettingsForWorkspace(r.Context(), contextGetWorkspace(r))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pageSize := queryCursorPageSize(input.PageSize, runtimeSettings)
	cursorID, ok := app.queryCursorParam(w, r)
	if !ok {
		return
	}

	out, err := app.runtime.Fetch(r.Context(), runtimeScope(r), execution.FetchRequest{
		SessionID: requestSessionID(r),
		CursorID:  cursorID,
		PageSize:  pageSize,
		Limits:    queryLimits(runtimeSettings),
	})
	if err != nil {
		var failure *execution.Failure
		if !errors.As(err, &failure) && !isRequestCanceled(r, err) {
			app.logWarn(r, "query cursor fetch failed",
				slog.String("query_cursor_id", string(cursorID)),
				slog.String("error", err.Error()),
			)
		}
		app.executionError(w, r, err)
		return
	}

	app.logDebug(r, "query cursor fetched",
		slog.String("query_cursor_id", string(cursorID)),
		slog.Int("page_size", pageSize),
		slog.Bool("exhausted", out.Exhausted),
	)
	out.CursorID = cursorID
	app.writeQueryCursorPage(w, r, out, pageSize)
}

// closeQueryCursor is idempotent: closing an unknown or foreign cursor
// answers 204 exactly like closing one's own.
func (app *application) closeQueryCursor(w http.ResponseWriter, r *http.Request) {
	cursorID, ok := app.queryCursorParam(w, r)
	if !ok {
		return
	}
	if err := app.runtime.CloseCursor(r.Context(), runtimeScope(r), requestSessionID(r), cursorID); err != nil {
		app.executionError(w, r, err)
		return
	}
	app.logDebug(r, "query cursor closed", slog.String("query_cursor_id", string(cursorID)))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) queryCursorParam(w http.ResponseWriter, r *http.Request) (execution.CursorID, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "query_cursor_id"))
	if id == "" {
		app.notFound(w, r)
		return "", false
	}
	return execution.CursorID(id), true
}

func queryCursorPageSize(requested *int, settings effectiveRuntimeSettings) int {
	pageSize := settings.QueryCursorPageSize
	if requested != nil {
		pageSize = *requested
	}
	if pageSize > settings.QueryMaxResultRows {
		return settings.QueryMaxResultRows
	}
	return pageSize
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

func (app *application) requiredConnectionRuntimePermission(r *http.Request, sql string) (string, bool, error) {
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	conn := contextGetConnection(r)

	if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnExecute) {
		return access.PermConnExecute, true, nil
	}

	classification, err := app.classifyConnectionSQL(r, conn, sql)
	if err != nil {
		return "", false, err
	}

	switch classification.Kind {
	case classifier.KindDQL:
		if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnDQL) {
			return access.PermConnDQL, true, nil
		}
	case classifier.KindDML:
		if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnDML) {
			return access.PermConnDML, true, nil
		}
	case classifier.KindDDL:
		if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnDDL) {
			return access.PermConnDDL, true, nil
		}
	}

	return "", false, nil
}
