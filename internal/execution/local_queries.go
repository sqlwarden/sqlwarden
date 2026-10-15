package execution

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/pkg/result"
)

const (
	// cursorIdleTimeout bounds how long an unfetched cursor stays open.
	cursorIdleTimeout = 30 * time.Minute

	defaultCursorPageSize = 200
	// maxCursorPageSize caps a page when the caller supplies no row ceiling.
	maxCursorPageSize = 10000
)

func pageSizeFor(requested int, limits Limits) int {
	size := requested
	if size <= 0 {
		size = defaultCursorPageSize
	}
	ceiling := maxCursorPageSize
	if limits.MaxRows > 0 {
		ceiling = limits.MaxRows
	}
	if size > ceiling {
		return ceiling
	}
	return size
}

func scanOptions(maxRows int, limits Limits) cursor.ScanOptions {
	return cursor.ScanOptions{MaxRows: maxRows, MaxBytes: limits.MaxBytes}
}

func txStatusOf(sess *connection.Session) TxStatus {
	st := sess.TransactionStatus()
	return TxStatus{
		Mode:              TxMode(st.Mode),
		Open:              st.Open,
		PendingStatements: st.PendingStatements,
		Statements:        st.Statements,
	}
}

// failed maps err for op. A statement interrupted by its context may still be
// running on the target connection, so a cancelled query or execution
// discards the session instead of returning a connection in an unknown state.
func (r *LocalRuntime) failed(ctx context.Context, sess *connection.Session, op operation, err error) error {
	if err == nil {
		return nil
	}
	if isContextError(err) || ctx.Err() != nil {
		if !isContextError(err) {
			err = errors.Join(ctx.Err(), err)
		}
		if op == opQuery || op == opExecute {
			r.manager.Remove(sess.ID)
		}
	}
	return mapError(err, op)
}

func (r *LocalRuntime) Query(ctx context.Context, scope Scope, req QueryRequest) (QueryResult, error) {
	sess, err := r.session(scope, req.SessionID)
	if err != nil {
		return QueryResult{}, mapError(err, opQuery)
	}
	start := time.Now()

	if req.Explain != nil {
		rs, err := sess.ExecuteExplainPlan(ctx, *req.Explain, scanOptions(req.Limits.MaxRows, req.Limits))
		r.runExplainTeardown(ctx, sess, req.Explain.Teardown)
		if err != nil {
			return QueryResult{}, r.failed(ctx, sess, opQuery, err)
		}
		rs.DurationMs = time.Since(start).Milliseconds()
		return QueryResult{Result: rs, Exhausted: true, Transaction: txStatusOf(sess)}, nil
	}

	if req.UseCursor {
		out, err := r.queryWithCursor(ctx, sess, req, start)
		if err == nil {
			return out, nil
		}
		if !errors.Is(err, connection.ErrQueryCursorsUnsupported) || req.RequireCursor {
			return QueryResult{}, r.failed(ctx, sess, opQuery, err)
		}
		r.logger.InfoContext(ctx, "query cursor unsupported; falling back to buffered query",
			slog.String("session_id", sess.ID))
	}

	rs, err := sess.QueryWithOptions(ctx, req.SQL, scanOptions(req.Limits.MaxRows, req.Limits), req.Args...)
	if err != nil {
		return QueryResult{}, r.failed(ctx, sess, opQuery, err)
	}
	rs.DurationMs = time.Since(start).Milliseconds()
	return QueryResult{Result: rs, Exhausted: true, Transaction: txStatusOf(sess)}, nil
}

func (r *LocalRuntime) runExplainTeardown(ctx context.Context, sess *connection.Session, teardown []string) {
	for _, stmt := range teardown {
		if _, err := sess.Execute(context.WithoutCancel(ctx), stmt); err != nil {
			r.logger.WarnContext(ctx, "explain teardown failed", slog.String("session_id", sess.ID))
		}
	}
}

func (r *LocalRuntime) queryWithCursor(ctx context.Context, sess *connection.Session, req QueryRequest, start time.Time) (QueryResult, error) {
	pageSize := pageSizeFor(req.PageSize, req.Limits)

	// database/sql ties Rows to the context that created them. The cursor must
	// outlive the operation that opened it, so creation is detached from the
	// caller's cancellation; Fetch, CloseCursor, session removal and the idle
	// reaper own cleanup afterwards.
	handle, err := sess.StartQueryCursor(context.WithoutCancel(ctx), req.SQL, req.Args...)
	if err != nil {
		return QueryResult{}, err
	}
	record := r.cursors.Create(connection.QueryCursorCreateParams{ParentSession: sess, Cursor: handle})

	rs, state, err := handle.Fetch(ctx, scanOptions(pageSize, req.Limits))
	if err != nil {
		r.cursors.Remove(record.ID)
		return QueryResult{}, err
	}
	rs.DurationMs = time.Since(start).Milliseconds()
	rs.PageSize = pageSize
	exhausted := state.Exhausted
	rs.Exhausted = &exhausted

	out := QueryResult{Result: rs, Exhausted: exhausted, Transaction: txStatusOf(sess)}
	if exhausted {
		record.MarkExhausted()
		r.cursors.Remove(record.ID)
	} else {
		out.CursorID = CursorID(record.ID)
		rs.QueryCursorID = record.ID
	}
	r.logger.DebugContext(ctx, "query cursor initial page returned",
		slog.String("session_id", sess.ID),
		slog.String("query_cursor_id", record.ID),
		slog.Int("page_size", pageSize),
		slog.Int("rows_returned", state.RowsReturned),
		slog.Int64("bytes_returned", state.BytesReturned),
		slog.Bool("exhausted", state.Exhausted),
		slog.Bool("truncated", rs.Truncated),
		slog.Int64("duration_ms", rs.DurationMs))
	return out, nil
}

// ownedCursor returns the cursor record when it exists and belongs to the
// given session within scope. An empty id matches any session in scope.
func (r *LocalRuntime) ownedCursor(scope Scope, id SessionID, cursorID CursorID) (*connection.QueryCursorRecord, bool) {
	record, ok := r.cursors.Get(string(cursorID))
	if !ok || record.ParentSession == nil ||
		(id != "" && record.ParentSession.ID != string(id)) || !ownedBy(record.ParentSession, scope) {
		return nil, false
	}
	return record, true
}

// cursorFor resolves a live cursor for scope. A cursor that is unknown or not
// owned by the caller's session is treated identically: the caller's own
// session is validated (session_not_found when it is not theirs), and only a
// proven-valid scope may learn cursor_lost. A caller therefore cannot tell a
// foreign cursor id from an unknown one. The owner of a cursor whose session
// has since closed gets cursor_lost because ownership is judged from the
// cursor's recorded parent scope. Without a session id there is no session to
// validate, so an unknown or foreign cursor is simply cursor_lost.
func (r *LocalRuntime) cursorFor(scope Scope, id SessionID, cursorID CursorID) (*connection.QueryCursorRecord, error) {
	record, owned := r.ownedCursor(scope, id, cursorID)
	if !owned {
		if id != "" {
			if _, err := r.session(scope, id); err != nil {
				return nil, err
			}
		}
		return nil, &Failure{Code: FailureCursorLost, Err: ErrCursorLost}
	}
	if live, ok := r.manager.Get(record.ParentSession.ID); !ok || live != record.ParentSession || !record.Touch() {
		r.cursors.Remove(record.ID)
		return nil, &Failure{Code: FailureCursorLost, Err: ErrCursorLost}
	}
	return record, nil
}

func (r *LocalRuntime) Fetch(ctx context.Context, scope Scope, req FetchRequest) (QueryResult, error) {
	record, err := r.cursorFor(scope, req.SessionID, req.CursorID)
	if err != nil {
		return QueryResult{}, err
	}
	pageSize := pageSizeFor(req.PageSize, req.Limits)
	start := time.Now()

	rs, state, err := record.Cursor.Fetch(ctx, scanOptions(pageSize, req.Limits))
	if err != nil {
		if !isContextError(err) && ctx.Err() == nil {
			r.cursors.Remove(record.ID)
		}
		return QueryResult{}, mapError(err, opFetch)
	}
	rs.DurationMs = time.Since(start).Milliseconds()
	rs.PageSize = pageSize
	exhausted := state.Exhausted
	rs.Exhausted = &exhausted

	out := QueryResult{Result: rs, Exhausted: exhausted}
	if exhausted {
		record.MarkExhausted()
		r.cursors.Remove(record.ID)
	} else {
		out.CursorID = req.CursorID
		rs.QueryCursorID = record.ID
	}
	r.logger.DebugContext(ctx, "query cursor fetched",
		slog.String("session_id", string(req.SessionID)),
		slog.String("query_cursor_id", record.ID),
		slog.Int("page_size", pageSize),
		slog.Int("rows_returned", state.RowsReturned),
		slog.Int64("bytes_returned", state.BytesReturned),
		slog.Bool("exhausted", state.Exhausted),
		slog.Bool("truncated", rs.Truncated),
		slog.Int64("duration_ms", rs.DurationMs))
	return out, nil
}

// CloseCursor is idempotent. A cursor that is unknown or owned by another
// scope is a no-op so the two cases are indistinguishable.
func (r *LocalRuntime) CloseCursor(ctx context.Context, scope Scope, id SessionID, cursorID CursorID) error {
	record, owned := r.ownedCursor(scope, id, cursorID)
	if !owned {
		return nil
	}
	removed := r.cursors.Remove(record.ID)
	r.logger.DebugContext(ctx, "query cursor closed",
		slog.String("session_id", string(id)),
		slog.String("query_cursor_id", record.ID),
		slog.Bool("removed", removed))
	return nil
}

func (r *LocalRuntime) Execute(ctx context.Context, scope Scope, req ExecuteRequest) (ExecuteResult, error) {
	if req.DDL != nil {
		return ExecuteResult{}, ErrDDLRequiresApply
	}
	sess, err := r.session(scope, req.SessionID)
	if err != nil {
		return ExecuteResult{}, mapError(err, opExecute)
	}
	start := time.Now()
	opts := scanOptions(req.Limits.MaxRows, req.Limits)

	var rs *result.ResultSet
	if req.Explain != nil {
		rs, err = sess.ExecuteExplainPlan(ctx, *req.Explain, opts)
		r.runExplainTeardown(ctx, sess, req.Explain.Teardown)
	} else {
		rs, err = sess.ExecuteWithOptions(ctx, req.SQL, opts, req.Args...)
	}
	if err != nil {
		return ExecuteResult{}, r.failed(ctx, sess, opExecute, err)
	}
	rs.DurationMs = time.Since(start).Milliseconds()
	return ExecuteResult{Result: rs, Transaction: txStatusOf(sess)}, nil
}

// Cancel discards the session, which interrupts anything running on it and
// closes its cursors and transaction. It is not wired to the HTTP cancel
// route, which only cancels the request context.
func (r *LocalRuntime) Cancel(ctx context.Context, scope Scope, id SessionID) error {
	return mapError(r.Close(ctx, scope, id), opCancel)
}

// Stream writes the statement's full result to w in req.Format. It reads
// through the session's driver on its own cursor without holding the session
// lock for the duration, so the caller must not run other statements on the
// session while it streams. Background exports open a private (Ephemeral)
// session for that reason; the synchronous download streams on its pooled
// session and relies on the request being the only user. Callers must classify
// req.SQL as a single read statement beforehand.
func (r *LocalRuntime) Stream(ctx context.Context, scope Scope, id SessionID, req StreamRequest, w io.Writer) (exports.StreamResult, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return exports.StreamResult{}, mapError(err, opStream)
	}
	touch := func() { r.manager.Get(sess.ID) }
	touch()

	res, err := exports.NewService().Stream(ctx, sess.Conn, w, exports.StreamOptions{
		Format:   req.Format,
		SQL:      req.SQL,
		MaxBytes: req.Limits.MaxBytes,
		OnProgress: func(rows, bytes int64) {
			touch()
			if req.OnProgress != nil {
				req.OnProgress(rows, bytes)
			}
		},
	})
	if err != nil {
		return res, mapError(err, opStream)
	}
	return res, nil
}
