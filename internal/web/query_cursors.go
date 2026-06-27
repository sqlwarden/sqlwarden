package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"github.com/sqlwarden/pkg/result"
)

const (
	defaultQueryCursorPageSize = 20

	apiErrorQueryCursorUnavailable = "query_cursor_unavailable"
)

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

func (app *application) queryCursorManager() *connection.QueryCursorManager {
	if app.queryCursors == nil {
		app.queryCursors = connection.NewQueryCursorManager(30 * time.Minute)
	}
	return app.queryCursors
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

	pageSize := app.queryCursorPageSize(input.PageSize)
	session, ok := app.resolveQueryRuntimeSession(w, r, input.SQL)
	if !ok {
		return
	}

	start := time.Now()
	cursor, err := session.StartQueryCursor(queryCursorLifetimeContext(r.Context()), input.SQL)
	if err != nil {
		if errors.Is(err, connection.ErrQueryCursorsUnsupported) {
			app.logWarn(r, "query cursor unsupported",
				slog.String("session_id", session.ID),
				slog.Int("page_size", pageSize),
			)
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query cursors.", nil)
			return
		}
		if app.isQueryRequestCanceled(r, err) {
			app.connManager.Remove(session.ID)
			app.logWarn(r, "query cursor start cancelled",
				slog.String("session_id", session.ID),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.logWarn(r, "query cursor start failed",
			slog.String("session_id", session.ID),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("error", err.Error()),
		)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	qc := app.queryCursorManager().Create(connection.QueryCursorCreateParams{
		ParentSession: session,
		Cursor:        cursor,
	})

	rs, state, err := cursor.Fetch(r.Context(), app.queryCursorScanOptions(pageSize))
	if err != nil {
		app.queryCursorManager().Remove(qc.ID)
		if app.isQueryRequestCanceled(r, err) {
			app.connManager.Remove(session.ID)
			app.logWarn(r, "query cursor initial fetch cancelled",
				queryCursorRecordAttrs(qc,
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				)...,
			)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.logWarn(r, "query cursor initial fetch failed",
			queryCursorRecordAttrs(qc,
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("error", err.Error()),
			)...,
		)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	if state.Exhausted {
		qc.MarkExhausted()
		app.queryCursorManager().Remove(qc.ID)
	}

	app.logInfo(r, "query cursor started",
		queryCursorRecordAttrs(qc,
			slog.Int("page_size", pageSize),
			slog.Int("rows_returned", state.RowsReturned),
			slog.Int64("bytes_returned", state.BytesReturned),
			slog.Bool("exhausted", state.Exhausted),
			slog.Bool("truncated", rs.Truncated),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)...,
	)

	app.writeQueryCursorPage(w, r, qc.ID, rs, state.Exhausted, pageSize, time.Since(start))
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

	pageSize := app.queryCursorPageSize(input.PageSize)
	qc, ok := app.resolveQueryCursorRecord(w, r)
	if !ok {
		return
	}
	if qc.ParentSession == nil {
		app.queryCursorManager().Remove(qc.ID)
		app.logWarn(r, "query cursor unavailable",
			queryCursorRecordAttrs(qc, slog.String("reason", "missing_parent_session"))...,
		)
		app.queryCursorUnavailable(w, r)
		return
	}
	if _, ok := app.connManager.Get(qc.ParentSession.ID); !ok {
		app.queryCursorManager().Remove(qc.ID)
		app.logWarn(r, "query cursor unavailable",
			queryCursorRecordAttrs(qc, slog.String("reason", "parent_session_not_found"))...,
		)
		app.queryCursorUnavailable(w, r)
		return
	}
	if !qc.Touch() {
		app.queryCursorManager().Remove(qc.ID)
		app.logWarn(r, "query cursor unavailable",
			queryCursorRecordAttrs(qc, slog.String("reason", "cursor_closed"))...,
		)
		app.queryCursorUnavailable(w, r)
		return
	}

	start := time.Now()
	rs, state, err := qc.Cursor.Fetch(r.Context(), app.queryCursorScanOptions(pageSize))
	if err != nil {
		if app.isQueryRequestCanceled(r, err) {
			app.logWarn(r, "query cursor fetch cancelled",
				queryCursorRecordAttrs(qc,
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				)...,
			)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.queryCursorManager().Remove(qc.ID)
		if errors.Is(err, cursor.ErrCursorClosed) {
			app.logWarn(r, "query cursor unavailable",
				queryCursorRecordAttrs(qc, slog.String("reason", "driver_cursor_closed"))...,
			)
			app.queryCursorUnavailable(w, r)
			return
		}
		app.logWarn(r, "query cursor fetch failed",
			queryCursorRecordAttrs(qc,
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("error", err.Error()),
			)...,
		)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	if state.Exhausted {
		qc.MarkExhausted()
		app.queryCursorManager().Remove(qc.ID)
	}

	app.logInfo(r, "query cursor fetched",
		queryCursorRecordAttrs(qc,
			slog.Int("page_size", pageSize),
			slog.Int("rows_returned", state.RowsReturned),
			slog.Int64("bytes_returned", state.BytesReturned),
			slog.Bool("exhausted", state.Exhausted),
			slog.Bool("truncated", rs.Truncated),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)...,
	)

	app.writeQueryCursorPage(w, r, qc.ID, rs, state.Exhausted, pageSize, time.Since(start))
}

func (app *application) closeQueryCursor(w http.ResponseWriter, r *http.Request) {
	qc, ok := app.resolveQueryCursorRecord(w, r)
	if !ok {
		return
	}
	removed := app.queryCursorManager().Remove(qc.ID)
	app.logInfo(r, "query cursor closed",
		queryCursorRecordAttrs(qc, slog.Bool("removed", removed))...,
	)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) resolveQueryCursorRecord(w http.ResponseWriter, r *http.Request) (*connection.QueryCursorRecord, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "query_cursor_id"))
	if id == "" {
		app.notFound(w, r)
		return nil, false
	}
	qc, ok := app.queryCursorManager().Get(id)
	if !ok {
		app.queryCursorUnavailable(w, r)
		return nil, false
	}
	if !app.queryCursorMatchesRequest(r, qc) {
		app.logWarn(r, "query cursor scope mismatch", queryCursorRecordAttrs(qc)...)
		app.notFound(w, r)
		return nil, false
	}
	return qc, true
}

func (app *application) queryCursorMatchesRequest(r *http.Request, qc *connection.QueryCursorRecord) bool {
	if qc.ParentSession == nil {
		return false
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	conn := contextGetConnection(r)
	return qc.ParentSession.AccountID == strconv.FormatInt(account.ID, 10) &&
		qc.ParentSession.OrgID == strconv.FormatInt(org.ID, 10) &&
		qc.ParentSession.WorkspaceID == strconv.FormatInt(ws.ID, 10) &&
		qc.ParentSession.ConnectionID == strconv.FormatInt(conn.ID, 10)
}

func queryCursorRecordAttrs(qc *connection.QueryCursorRecord, attrs ...slog.Attr) []slog.Attr {
	closed, exhausted := qc.State()
	out := []slog.Attr{
		slog.String("query_cursor_id", qc.ID),
		slog.String("session_id", qc.ParentSessionID),
		slog.String("driver_cursor_id", qc.CursorID),
		slog.Bool("closed", closed),
		slog.Bool("exhausted", exhausted),
	}
	if qc.ParentSession != nil {
		out = append(out,
			slog.String("session_account_id", qc.ParentSession.AccountID),
			slog.String("session_org_id", qc.ParentSession.OrgID),
			slog.String("session_workspace_id", qc.ParentSession.WorkspaceID),
			slog.String("session_connection_id", qc.ParentSession.ConnectionID),
		)
	}
	return append(out, attrs...)
}

func (app *application) queryCursorPageSize(requested *int) int {
	pageSize := defaultQueryCursorPageSize
	if requested != nil {
		pageSize = *requested
	}
	if pageSize > app.config.Query.MaxResultRows {
		return app.config.Query.MaxResultRows
	}
	return pageSize
}

func (app *application) queryCursorScanOptions(pageSize int) cursor.ScanOptions {
	return cursor.ScanOptions{
		MaxRows:  pageSize,
		MaxBytes: int64(app.config.Query.MaxResultBytes),
	}
}

func (app *application) writeQueryCursorPage(w http.ResponseWriter, r *http.Request, queryCursorID string, rs *result.ResultSet, exhausted bool, pageSize int, duration time.Duration) {
	if rs == nil {
		rs = &result.ResultSet{}
	}
	payload := queryCursorPageResponse{
		QueryCursorID:    queryCursorID,
		Columns:          rs.Columns,
		Rows:             rs.Rows,
		DurationMs:       duration.Milliseconds(),
		Truncated:        rs.Truncated,
		RowsReturned:     rs.RowsReturned,
		BytesReturned:    rs.BytesReturned,
		TruncationReason: rs.TruncationReason,
		Exhausted:        exhausted,
		PageSize:         pageSize,
	}
	if err := response.JSON(w, http.StatusOK, payload); err != nil {
		app.serverError(w, r, err)
	}
}

func queryCursorLifetimeContext(ctx context.Context) context.Context {
	// database/sql ties Rows to the QueryContext used to create them. A query
	// cursor must survive beyond the HTTP request that opened it, so detach it
	// from request cancellation; explicit close, exhaustion, parent session
	// removal, and the idle reaper own cursor cleanup after creation.
	return context.WithoutCancel(ctx)
}

func (app *application) queryCursorUnavailable(w http.ResponseWriter, r *http.Request) {
	app.apiError(w, r, http.StatusGone, apiErrorQueryCursorUnavailable, "Query cursor is no longer available. Run the query again.", response.APIError{}, nil)
}

func (app *application) isQueryRequestCanceled(r *http.Request, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || r.Context().Err() != nil
}

func (app *application) resolveQueryRuntimeSession(w http.ResponseWriter, r *http.Request, sql string) (*connection.Session, bool) {
	account := contextGetAccount(r)
	conn := contextGetConnection(r)

	_, allowed, err := app.requiredConnectionRuntimePermission(r, sql)
	if err != nil {
		app.serverError(w, r, err)
		return nil, false
	}
	if !allowed {
		app.notPermitted(w, r)
		return nil, false
	}

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return nil, false
	}
	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return nil, false
	}
	if session.AccountID != strconv.FormatInt(account.ID, 10) || session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return nil, false
	}
	return session, true
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
