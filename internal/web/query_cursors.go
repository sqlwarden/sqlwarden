package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/sqlquery"
	"github.com/sqlwarden/internal/validator"
	"github.com/sqlwarden/pkg/result"
)

const (
	defaultQueryCursorPageSize = 500

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

type queryCursorCreateParams struct {
	AccountID          int64
	OrgID              int64
	WorkspaceID        int64
	EnvironmentID      int64
	ConnectionID       int64
	ParentSessionID    string
	ParentSession      *connection.Session
	Cursor             *connection.QueryCursorHandle
	RequiredPermission string
}

type queryCursor struct {
	ID                 string
	AccountID          int64
	OrgID              int64
	WorkspaceID        int64
	EnvironmentID      int64
	ConnectionID       int64
	ParentSessionID    string
	ParentSession      *connection.Session
	CursorID           string
	Cursor             *connection.QueryCursorHandle
	RequiredPermission string
	CreatedAt          time.Time
	LastUsedAt         time.Time
	Closed             bool
	Exhausted          bool
	mu                 sync.Mutex
}

type queryCursorManager struct {
	mu          sync.RWMutex
	cursors     map[string]*queryCursor
	idleTimeout time.Duration
	stop        chan struct{}
	stopped     chan struct{}
	closeOnce   sync.Once
}

func newQueryCursorManager(idleTimeout time.Duration) *queryCursorManager {
	m := &queryCursorManager{
		cursors:     make(map[string]*queryCursor),
		idleTimeout: idleTimeout,
		stop:        make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	go m.reap()
	return m
}

func (m *queryCursorManager) Create(params queryCursorCreateParams) *queryCursor {
	now := time.Now()
	qc := &queryCursor{
		ID:                 ulid.Make().String(),
		AccountID:          params.AccountID,
		OrgID:              params.OrgID,
		WorkspaceID:        params.WorkspaceID,
		EnvironmentID:      params.EnvironmentID,
		ConnectionID:       params.ConnectionID,
		ParentSessionID:    params.ParentSessionID,
		ParentSession:      params.ParentSession,
		CursorID:           params.Cursor.ID,
		Cursor:             params.Cursor,
		RequiredPermission: params.RequiredPermission,
		CreatedAt:          now,
		LastUsedAt:         now,
	}

	m.mu.Lock()
	m.cursors[qc.ID] = qc
	m.mu.Unlock()
	return qc
}

func (m *queryCursorManager) Get(id string) (*queryCursor, bool) {
	m.mu.RLock()
	qc, ok := m.cursors[id]
	m.mu.RUnlock()
	return qc, ok
}

func (m *queryCursorManager) Remove(id string) bool {
	qc, ok := m.take(id)
	if !ok {
		return false
	}
	return qc.close()
}

func (m *queryCursorManager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
	})
	<-m.stopped

	m.mu.Lock()
	cursors := make([]*queryCursor, 0, len(m.cursors))
	for id, qc := range m.cursors {
		cursors = append(cursors, qc)
		delete(m.cursors, id)
	}
	m.mu.Unlock()

	for _, qc := range cursors {
		qc.close()
	}
}

func (m *queryCursorManager) take(id string) (*queryCursor, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	qc, ok := m.cursors[id]
	if ok {
		delete(m.cursors, id)
	}
	return qc, ok
}

func (m *queryCursorManager) reap() {
	defer close(m.stopped)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-ticker.C:
			m.reapIdle(now)
		}
	}
}

func (m *queryCursorManager) reapIdle(now time.Time) int {
	m.mu.Lock()
	expired := make([]*queryCursor, 0)
	for id, qc := range m.cursors {
		qc.mu.Lock()
		expiredCursor := qc.Closed || now.Sub(qc.LastUsedAt) > m.idleTimeout
		qc.mu.Unlock()
		if expiredCursor {
			expired = append(expired, qc)
			delete(m.cursors, id)
		}
	}
	m.mu.Unlock()

	for _, qc := range expired {
		qc.close()
	}
	return len(expired)
}

func (qc *queryCursor) touch() bool {
	qc.mu.Lock()
	defer qc.mu.Unlock()
	if qc.Closed {
		return false
	}
	qc.LastUsedAt = time.Now()
	return true
}

func (qc *queryCursor) close() bool {
	qc.mu.Lock()
	if qc.Closed {
		qc.mu.Unlock()
		return false
	}
	qc.Closed = true
	qc.mu.Unlock()

	if qc.ParentSession != nil {
		_ = qc.ParentSession.CloseCursor(qc.CursorID)
	} else if qc.Cursor != nil {
		_ = qc.Cursor.Close()
	}
	return true
}

func (app *application) queryCursorManager() *queryCursorManager {
	if app.queryCursors == nil {
		app.queryCursors = newQueryCursorManager(30 * time.Minute)
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
	session, requiredPermission, ok := app.resolveQueryRuntimeSession(w, r, input.SQL)
	if !ok {
		return
	}

	start := time.Now()
	cursor, err := session.StartQueryCursor(r.Context(), input.SQL)
	if err != nil {
		if errors.Is(err, connection.ErrQueryCursorsUnsupported) {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query cursors.", nil)
			return
		}
		if app.isQueryRequestCanceled(r, err) {
			app.connManager.Remove(session.ID)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	env := contextGetEnvironment(r)
	conn := contextGetConnection(r)
	qc := app.queryCursorManager().Create(queryCursorCreateParams{
		AccountID:          account.ID,
		OrgID:              org.ID,
		WorkspaceID:        ws.ID,
		EnvironmentID:      env.ID,
		ConnectionID:       conn.ID,
		ParentSessionID:    session.ID,
		ParentSession:      session,
		Cursor:             cursor,
		RequiredPermission: requiredPermission,
	})

	rs, state, err := cursor.Fetch(r.Context(), app.queryCursorScanOptions(pageSize))
	if err != nil {
		app.queryCursorManager().Remove(qc.ID)
		if app.isQueryRequestCanceled(r, err) {
			app.connManager.Remove(session.ID)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	if state.Exhausted {
		app.queryCursorManager().Remove(qc.ID)
	}

	app.logger.Info("query cursor started",
		slog.String("query_cursor_id", qc.ID),
		slog.Int64("account_id", account.ID),
		slog.Int64("org_id", org.ID),
		slog.Int64("workspace_id", ws.ID),
		slog.Int64("connection_id", conn.ID),
		slog.Int("rows_returned", state.RowsReturned),
		slog.Int64("bytes_returned", state.BytesReturned),
		slog.Bool("exhausted", state.Exhausted),
		slog.Bool("truncated", rs.Truncated),
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
	if !app.canUseQueryCursorPermission(r, qc) {
		app.queryCursorManager().Remove(qc.ID)
		app.notPermitted(w, r)
		return
	}
	if _, ok := app.connManager.Get(qc.ParentSessionID); !ok {
		app.queryCursorManager().Remove(qc.ID)
		app.queryCursorUnavailable(w, r)
		return
	}
	if !qc.touch() {
		app.queryCursorManager().Remove(qc.ID)
		app.queryCursorUnavailable(w, r)
		return
	}

	start := time.Now()
	rs, state, err := qc.Cursor.Fetch(r.Context(), app.queryCursorScanOptions(pageSize))
	if err != nil {
		app.queryCursorManager().Remove(qc.ID)
		if app.isQueryRequestCanceled(r, err) {
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		if errors.Is(err, driver.ErrCursorClosed) {
			app.queryCursorUnavailable(w, r)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	if state.Exhausted {
		app.queryCursorManager().Remove(qc.ID)
	}

	app.logger.Info("query cursor fetched",
		slog.String("query_cursor_id", qc.ID),
		slog.Int64("account_id", qc.AccountID),
		slog.Int64("org_id", qc.OrgID),
		slog.Int64("workspace_id", qc.WorkspaceID),
		slog.Int64("connection_id", qc.ConnectionID),
		slog.Int("rows_returned", state.RowsReturned),
		slog.Int64("bytes_returned", state.BytesReturned),
		slog.Bool("exhausted", state.Exhausted),
		slog.Bool("truncated", rs.Truncated),
	)

	app.writeQueryCursorPage(w, r, qc.ID, rs, state.Exhausted, pageSize, time.Since(start))
}

func (app *application) closeQueryCursor(w http.ResponseWriter, r *http.Request) {
	qc, ok := app.resolveQueryCursorRecord(w, r)
	if !ok {
		return
	}
	app.queryCursorManager().Remove(qc.ID)
	app.logger.Info("query cursor closed",
		slog.String("query_cursor_id", qc.ID),
		slog.Int64("account_id", qc.AccountID),
		slog.Int64("org_id", qc.OrgID),
		slog.Int64("workspace_id", qc.WorkspaceID),
		slog.Int64("connection_id", qc.ConnectionID),
	)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) resolveQueryCursorRecord(w http.ResponseWriter, r *http.Request) (*queryCursor, bool) {
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
		app.notFound(w, r)
		return nil, false
	}
	return qc, true
}

func (app *application) queryCursorMatchesRequest(r *http.Request, qc *queryCursor) bool {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	env := contextGetEnvironment(r)
	conn := contextGetConnection(r)
	return qc.AccountID == account.ID &&
		qc.OrgID == org.ID &&
		qc.WorkspaceID == ws.ID &&
		qc.EnvironmentID == env.ID &&
		qc.ConnectionID == conn.ID
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

func (app *application) queryCursorScanOptions(pageSize int) driver.ScanOptions {
	return driver.ScanOptions{
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

func (app *application) queryCursorUnavailable(w http.ResponseWriter, r *http.Request) {
	app.apiError(w, r, http.StatusGone, apiErrorQueryCursorUnavailable, "Query cursor is no longer available. Run the query again.", response.APIError{}, nil)
}

func (app *application) canUseQueryCursorPermission(r *http.Request, qc *queryCursor) bool {
	if qc.RequiredPermission == "" {
		return false
	}
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)
	return app.enforcer.Can(r.Context(), account.ID, org.ID, ws.OwnerType, "connection", qc.ConnectionID, qc.RequiredPermission)
}

func (app *application) isQueryRequestCanceled(r *http.Request, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || r.Context().Err() != nil
}

func (app *application) resolveQueryRuntimeSession(w http.ResponseWriter, r *http.Request, sql string) (*connection.Session, string, bool) {
	account := contextGetAccount(r)
	conn := contextGetConnection(r)

	requiredPermission, allowed, err := app.requiredConnectionRuntimePermission(r, sql)
	if err != nil {
		app.serverError(w, r, err)
		return nil, "", false
	}
	if !allowed {
		app.notPermitted(w, r)
		return nil, "", false
	}

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return nil, "", false
	}
	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return nil, "", false
	}
	if session.AccountID != strconv.FormatInt(account.ID, 10) || session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return nil, "", false
	}
	return session, requiredPermission, true
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
	case sqlquery.KindDQL:
		if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnDQL) {
			return access.PermConnDQL, true, nil
		}
	case sqlquery.KindDML:
		if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnDML) {
			return access.PermConnDML, true, nil
		}
	case sqlquery.KindDDL:
		if app.hasConnectionPermission(r, org.ID, ws.OwnerType, conn.ID, access.PermConnDDL) {
			return access.PermConnDDL, true, nil
		}
	}

	return "", false, nil
}
