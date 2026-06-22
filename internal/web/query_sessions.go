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
	defaultQuerySessionPageSize = 500

	apiErrorQuerySessionUnavailable = "query_session_unavailable"
)

type querySessionRequest struct {
	SQL      string              `json:"sql"`
	PageSize *int                `json:"page_size"`
	V        validator.Validator `json:"-"`
}

type querySessionFetchRequest struct {
	PageSize *int `json:"page_size"`
}

type querySessionPageResponse struct {
	QuerySessionID   string          `json:"query_session_id"`
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

type querySessionCreateParams struct {
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

type querySession struct {
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

type querySessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*querySession
	idleTimeout time.Duration
	stop        chan struct{}
	stopped     chan struct{}
	closeOnce   sync.Once
}

func newQuerySessionManager(idleTimeout time.Duration) *querySessionManager {
	m := &querySessionManager{
		sessions:    make(map[string]*querySession),
		idleTimeout: idleTimeout,
		stop:        make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	go m.reap()
	return m
}

func (m *querySessionManager) Create(params querySessionCreateParams) *querySession {
	now := time.Now()
	qs := &querySession{
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
	m.sessions[qs.ID] = qs
	m.mu.Unlock()
	return qs
}

func (m *querySessionManager) Get(id string) (*querySession, bool) {
	m.mu.RLock()
	qs, ok := m.sessions[id]
	m.mu.RUnlock()
	return qs, ok
}

func (m *querySessionManager) Remove(id string) bool {
	qs, ok := m.take(id)
	if !ok {
		return false
	}
	return qs.close()
}

func (m *querySessionManager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
	})
	<-m.stopped

	m.mu.Lock()
	sessions := make([]*querySession, 0, len(m.sessions))
	for id, qs := range m.sessions {
		sessions = append(sessions, qs)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, qs := range sessions {
		qs.close()
	}
}

func (m *querySessionManager) take(id string) (*querySession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	qs, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	return qs, ok
}

func (m *querySessionManager) reap() {
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

func (m *querySessionManager) reapIdle(now time.Time) int {
	m.mu.Lock()
	expired := make([]*querySession, 0)
	for id, qs := range m.sessions {
		qs.mu.Lock()
		expiredSession := qs.Closed || now.Sub(qs.LastUsedAt) > m.idleTimeout
		qs.mu.Unlock()
		if expiredSession {
			expired = append(expired, qs)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()

	for _, qs := range expired {
		qs.close()
	}
	return len(expired)
}

func (qs *querySession) touch() bool {
	qs.mu.Lock()
	defer qs.mu.Unlock()
	if qs.Closed {
		return false
	}
	qs.LastUsedAt = time.Now()
	return true
}

func (qs *querySession) close() bool {
	qs.mu.Lock()
	if qs.Closed {
		qs.mu.Unlock()
		return false
	}
	qs.Closed = true
	qs.mu.Unlock()

	if qs.ParentSession != nil {
		_ = qs.ParentSession.CloseCursor(qs.CursorID)
	} else if qs.Cursor != nil {
		_ = qs.Cursor.Close()
	}
	return true
}

func (app *application) querySessionManager() *querySessionManager {
	if app.querySessions == nil {
		app.querySessions = newQuerySessionManager(30 * time.Minute)
	}
	return app.querySessions
}

func (app *application) startQuerySession(w http.ResponseWriter, r *http.Request) {
	var input querySessionRequest
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

	pageSize := app.querySessionPageSize(input.PageSize)
	session, requiredPermission, ok := app.resolveQueryRuntimeSession(w, r, input.SQL)
	if !ok {
		return
	}

	start := time.Now()
	cursor, err := session.StartQueryCursor(r.Context(), input.SQL)
	if err != nil {
		if errors.Is(err, connection.ErrQueryCursorsUnsupported) {
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query sessions.", nil)
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
	qs := app.querySessionManager().Create(querySessionCreateParams{
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

	rs, state, err := cursor.Fetch(r.Context(), app.querySessionScanOptions(pageSize))
	if err != nil {
		app.querySessionManager().Remove(qs.ID)
		if app.isQueryRequestCanceled(r, err) {
			app.connManager.Remove(session.ID)
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}

	if state.Exhausted {
		app.querySessionManager().Remove(qs.ID)
	}

	app.logger.Info("query session started",
		slog.String("query_session_id", qs.ID),
		slog.Int64("account_id", account.ID),
		slog.Int64("org_id", org.ID),
		slog.Int64("workspace_id", ws.ID),
		slog.Int64("connection_id", conn.ID),
		slog.Int("rows_returned", state.RowsReturned),
		slog.Int64("bytes_returned", state.BytesReturned),
		slog.Bool("exhausted", state.Exhausted),
		slog.Bool("truncated", rs.Truncated),
	)

	app.writeQuerySessionPage(w, r, qs.ID, rs, state.Exhausted, pageSize, time.Since(start))
}

func (app *application) fetchQuerySession(w http.ResponseWriter, r *http.Request) {
	var input querySessionFetchRequest
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

	pageSize := app.querySessionPageSize(input.PageSize)
	qs, ok := app.resolveQuerySessionRecord(w, r)
	if !ok {
		return
	}
	if !app.canUseQuerySessionPermission(r, qs) {
		app.querySessionManager().Remove(qs.ID)
		app.notPermitted(w, r)
		return
	}
	if _, ok := app.connManager.Get(qs.ParentSessionID); !ok {
		app.querySessionManager().Remove(qs.ID)
		app.querySessionUnavailable(w, r)
		return
	}
	if !qs.touch() {
		app.querySessionManager().Remove(qs.ID)
		app.querySessionUnavailable(w, r)
		return
	}

	start := time.Now()
	rs, state, err := qs.Cursor.Fetch(r.Context(), app.querySessionScanOptions(pageSize))
	if err != nil {
		app.querySessionManager().Remove(qs.ID)
		if app.isQueryRequestCanceled(r, err) {
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		}
		if errors.Is(err, driver.ErrCursorClosed) {
			app.querySessionUnavailable(w, r)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	if state.Exhausted {
		app.querySessionManager().Remove(qs.ID)
	}

	app.logger.Info("query session fetched",
		slog.String("query_session_id", qs.ID),
		slog.Int64("account_id", qs.AccountID),
		slog.Int64("org_id", qs.OrgID),
		slog.Int64("workspace_id", qs.WorkspaceID),
		slog.Int64("connection_id", qs.ConnectionID),
		slog.Int("rows_returned", state.RowsReturned),
		slog.Int64("bytes_returned", state.BytesReturned),
		slog.Bool("exhausted", state.Exhausted),
		slog.Bool("truncated", rs.Truncated),
	)

	app.writeQuerySessionPage(w, r, qs.ID, rs, state.Exhausted, pageSize, time.Since(start))
}

func (app *application) closeQuerySession(w http.ResponseWriter, r *http.Request) {
	qs, ok := app.resolveQuerySessionRecord(w, r)
	if !ok {
		return
	}
	app.querySessionManager().Remove(qs.ID)
	app.logger.Info("query session closed",
		slog.String("query_session_id", qs.ID),
		slog.Int64("account_id", qs.AccountID),
		slog.Int64("org_id", qs.OrgID),
		slog.Int64("workspace_id", qs.WorkspaceID),
		slog.Int64("connection_id", qs.ConnectionID),
	)
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) resolveQuerySessionRecord(w http.ResponseWriter, r *http.Request) (*querySession, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "query_session_id"))
	if id == "" {
		app.notFound(w, r)
		return nil, false
	}
	qs, ok := app.querySessionManager().Get(id)
	if !ok {
		app.querySessionUnavailable(w, r)
		return nil, false
	}
	if !app.querySessionMatchesRequest(r, qs) {
		app.notFound(w, r)
		return nil, false
	}
	return qs, true
}

func (app *application) querySessionMatchesRequest(r *http.Request, qs *querySession) bool {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	env := contextGetEnvironment(r)
	conn := contextGetConnection(r)
	return qs.AccountID == account.ID &&
		qs.OrgID == org.ID &&
		qs.WorkspaceID == ws.ID &&
		qs.EnvironmentID == env.ID &&
		qs.ConnectionID == conn.ID
}

func (app *application) querySessionPageSize(requested *int) int {
	pageSize := defaultQuerySessionPageSize
	if requested != nil {
		pageSize = *requested
	}
	if pageSize > app.config.Query.MaxResultRows {
		return app.config.Query.MaxResultRows
	}
	return pageSize
}

func (app *application) querySessionScanOptions(pageSize int) driver.ScanOptions {
	return driver.ScanOptions{
		MaxRows:  pageSize,
		MaxBytes: int64(app.config.Query.MaxResultBytes),
	}
}

func (app *application) writeQuerySessionPage(w http.ResponseWriter, r *http.Request, querySessionID string, rs *result.ResultSet, exhausted bool, pageSize int, duration time.Duration) {
	if rs == nil {
		rs = &result.ResultSet{}
	}
	payload := querySessionPageResponse{
		QuerySessionID:   querySessionID,
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

func (app *application) querySessionUnavailable(w http.ResponseWriter, r *http.Request) {
	app.apiError(w, r, http.StatusGone, apiErrorQuerySessionUnavailable, "Query session is no longer available. Run the query again.", response.APIError{}, nil)
}

func (app *application) canUseQuerySessionPermission(r *http.Request, qs *querySession) bool {
	if qs.RequiredPermission == "" {
		return false
	}
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)
	return app.enforcer.Can(r.Context(), account.ID, org.ID, ws.OwnerType, "connection", qs.ConnectionID, qs.RequiredPermission)
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
