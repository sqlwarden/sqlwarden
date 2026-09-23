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
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	settingsapp "github.com/sqlwarden/internal/settings"
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

	runtimeSettings, err := app.settingsService().EffectiveForWorkspace(r.Context(), contextGetWorkspace(r))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pageSize := queryCursorPageSize(input.PageSize, runtimeSettings)
	handle, ok := app.resolveQueryRuntimeSession(w, r, input.SQL)
	if !ok {
		return
	}

	start := time.Now()
	query, err := app.executionRuntime.Query(r.Context(), execution.QueryRequest{
		Handle: handle, SQL: input.SQL, UseCursor: true, PageSize: pageSize,
		Limits: execution.Limits{MaxRows: pageSize, MaxBytes: runtimeSettings.QueryMaxResultBytes},
	})
	if err != nil {
		if errors.Is(err, execution.ErrQueryCursorUnsupported) {
			app.logWarn(r, "query cursor unsupported",
				slog.Int("page_size", pageSize),
			)
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query cursors.", nil)
			return
		}
		if app.isQueryRequestCanceled(r, err) {
			_ = app.executionRuntime.Cancel(context.WithoutCancel(r.Context()), execution.SessionRequest{Handle: handle})
			app.logDebug(r, "query cursor start cancelled",
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.logWarn(r, "query cursor start failed",
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("failure_category", executionErrorCategory(err)),
		)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	app.logDebug(r, "query cursor started",
		queryCursorAttrs(handle, query.Cursor,
			slog.Int("page_size", pageSize),
			slog.Int("rows_returned", query.Result.RowsReturned),
			slog.Int64("bytes_returned", query.Result.BytesReturned),
			slog.Bool("exhausted", query.Exhausted),
			slog.Bool("truncated", query.Result.Truncated),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)...,
	)

	app.writeQueryCursorPage(w, r, string(query.Cursor), query.Result, query.Exhausted, pageSize, time.Since(start))
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

	runtimeSettings, err := app.settingsService().EffectiveForWorkspace(r.Context(), contextGetWorkspace(r))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	pageSize := queryCursorPageSize(input.PageSize, runtimeSettings)
	handle, cursorHandle, ok := app.resolveQueryCursor(w, r)
	if !ok {
		return
	}

	start := time.Now()
	fetched, err := app.executionRuntime.Fetch(r.Context(), execution.FetchRequest{
		Handle: handle, Cursor: cursorHandle, PageSize: pageSize,
		Limits: execution.Limits{MaxRows: pageSize, MaxBytes: runtimeSettings.QueryMaxResultBytes},
	})
	if err != nil {
		if app.isQueryRequestCanceled(r, err) {
			app.logDebug(r, "query cursor fetch cancelled",
				queryCursorAttrs(handle, cursorHandle,
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				)...,
			)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		if errors.Is(err, execution.ErrCursorLost) || errors.Is(err, cursor.ErrCursorClosed) {
			app.logWarn(r, "query cursor unavailable",
				queryCursorAttrs(handle, cursorHandle, slog.String("reason", "cursor_lost"))...,
			)
			app.queryCursorUnavailable(w, r)
			return
		}
		app.logWarn(r, "query cursor fetch failed",
			queryCursorAttrs(handle, cursorHandle,
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("failure_category", executionErrorCategory(err)),
			)...,
		)
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	app.logDebug(r, "query cursor fetched",
		queryCursorAttrs(handle, cursorHandle,
			slog.Int("page_size", pageSize),
			slog.Int("rows_returned", fetched.Result.RowsReturned),
			slog.Int64("bytes_returned", fetched.Result.BytesReturned),
			slog.Bool("exhausted", fetched.Exhausted),
			slog.Bool("truncated", fetched.Result.Truncated),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)...,
	)

	app.writeQueryCursorPage(w, r, string(cursorHandle), fetched.Result, fetched.Exhausted, pageSize, time.Since(start))
}

func (app *application) closeQueryCursor(w http.ResponseWriter, r *http.Request) {
	handle, cursorHandle, ok := app.resolveQueryCursor(w, r)
	if !ok {
		return
	}
	err := app.executionRuntime.Close(r.Context(), execution.CloseRequest{Handle: handle, Cursor: cursorHandle})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logDebug(r, "query cursor closed",
		queryCursorAttrs(handle, cursorHandle)...,
	)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) resolveQueryCursor(w http.ResponseWriter, r *http.Request) (execution.SessionHandle, execution.CursorHandle, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "query_cursor_id"))
	if id == "" {
		app.notFound(w, r)
		return "", "", false
	}
	sessionID := strings.TrimSpace(r.Header.Get("X-Warden-Session"))
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return "", "", false
	}
	handle := execution.SessionHandle(sessionID)
	info, found, err := app.executionRuntime.Session(r.Context(), handle)
	if err != nil {
		app.serverError(w, r, err)
		return "", "", false
	}
	if !found {
		app.queryCursorUnavailable(w, r)
		return "", "", false
	}
	if !querySessionMatchesRequest(r, info) {
		app.logWarn(r, "query cursor scope mismatch", queryCursorAttrs(handle, execution.CursorHandle(id))...)
		app.notFound(w, r)
		return "", "", false
	}
	return handle, execution.CursorHandle(id), true
}

func querySessionMatchesRequest(r *http.Request, session execution.SessionInfo) bool {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	conn := contextGetConnection(r)
	return session.Scope.AccountID == strconv.FormatInt(account.ID, 10) &&
		session.Scope.TenantID == strconv.FormatInt(org.ID, 10) &&
		session.Scope.WorkspaceID == strconv.FormatInt(ws.ID, 10) &&
		session.Scope.ConnectionID == strconv.FormatInt(conn.ID, 10)
}

func queryCursorAttrs(_ execution.SessionHandle, _ execution.CursorHandle, attrs ...slog.Attr) []slog.Attr {
	return attrs
}

func queryCursorPageSize(requested *int, settings settingsapp.Effective) int {
	pageSize := settings.QueryCursorPageSize
	if requested != nil {
		pageSize = *requested
	}
	if pageSize > settings.QueryMaxResultRows {
		return settings.QueryMaxResultRows
	}
	return pageSize
}

func queryCursorScanOptions(pageSize int, settings settingsapp.Effective) cursor.ScanOptions {
	return cursor.ScanOptions{
		MaxRows:  pageSize,
		MaxBytes: settings.QueryMaxResultBytes,
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

func (app *application) resolveQueryRuntimeSession(w http.ResponseWriter, r *http.Request, sql string) (execution.SessionHandle, bool) {
	account := contextGetAccount(r)
	conn := contextGetConnection(r)

	_, allowed, err := app.requiredConnectionRuntimePermission(r, sql)
	if err != nil {
		app.serverError(w, r, err)
		return "", false
	}
	if !allowed {
		app.notPermitted(w, r)
		return "", false
	}

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return "", false
	}
	handle := execution.SessionHandle(sessionID)
	session, found, err := app.executionRuntime.Session(r.Context(), handle)
	if err != nil {
		app.serverError(w, r, err)
		return "", false
	}
	if !found {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return "", false
	}
	if session.Scope.AccountID != strconv.FormatInt(account.ID, 10) || session.Scope.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return "", false
	}
	return handle, true
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
