package query

import (
	"context"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/execution"
)

// CursorInput starts a forward-only cursor over a read statement.
type CursorInput struct {
	SessionID execution.SessionID
	SQL       string
	PageSize  *int
}

// FetchInput continues a cursor. SessionID may be empty: the cursor id alone
// addresses the cursor, which must still belong to a session in the
// principal's scope.
type FetchInput struct {
	SessionID execution.SessionID
	CursorID  execution.CursorID
	PageSize  *int
}

// CursorPage is one page of a cursor together with the effective page size.
type CursorPage struct {
	Result   execution.QueryResult
	PageSize int
}

// StartCursor runs a statement through a required cursor and returns its
// first page. Runtime failures are wrapped in *RuntimeError and the returned
// page still carries the effective page size.
func (s *Service) StartCursor(ctx context.Context, p access.Principal, ref access.ConnRef, in CursorInput) (CursorPage, error) {
	conn, ws, err := s.runtimeConnection(ctx, p, ref)
	if err != nil {
		return CursorPage{}, err
	}
	fields := map[string]string{}
	if strings.TrimSpace(in.SQL) == "" {
		fields["sql"] = "SQL is required."
	}
	if in.PageSize != nil && *in.PageSize <= 0 {
		fields["page_size"] = "Page size must be greater than 0."
	}
	if len(fields) > 0 {
		return CursorPage{}, &ValidationError{Fields: fields}
	}
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return CursorPage{}, err
	}
	pageSize := cursorPageSize(in.PageSize, policy)

	// Broad execute covers every class, so the statement is classified only
	// when the decision depends on it.
	if !s.can(ctx, p, ref, ws, conn, access.PermConnExecute) {
		classification, err := s.deps.Analyzers.Classifier(conn.Driver).Classify(ctx, classifier.Request{SQL: in.SQL})
		if err != nil {
			return CursorPage{}, err
		}
		if _, allowed := s.kindPermission(ctx, p, ref, ws, conn, classification.Kind, false); !allowed {
			return CursorPage{}, ErrForbidden
		}
	}
	if in.SessionID == "" {
		return CursorPage{}, ErrSessionRequired
	}

	out, err := s.deps.Runtime.Query(ctx, scope(p, ref), execution.QueryRequest{
		SessionID:     in.SessionID,
		SQL:           in.SQL,
		Limits:        limits(policy),
		UseCursor:     true,
		RequireCursor: true,
		PageSize:      pageSize,
	})
	if err != nil {
		return CursorPage{PageSize: pageSize}, &RuntimeError{Err: err}
	}
	return CursorPage{Result: out, PageSize: pageSize}, nil
}

// FetchCursor returns the next page of a cursor. The runtime binds a cursor
// to the session that opened it, so the statement permission is not
// re-checked: it was authorized when the cursor started. The session-use gate
// still applies, so a principal whose access was revoked cannot keep paging.
func (s *Service) FetchCursor(ctx context.Context, p access.Principal, ref access.ConnRef, in FetchInput) (CursorPage, error) {
	_, ws, err := s.runtimeConnection(ctx, p, ref)
	if err != nil {
		return CursorPage{}, err
	}
	if in.PageSize != nil && *in.PageSize <= 0 {
		return CursorPage{}, &ValidationError{Fields: map[string]string{"page_size": "Page size must be greater than 0."}}
	}
	if in.CursorID == "" {
		return CursorPage{}, ErrNotFound
	}
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return CursorPage{}, err
	}
	pageSize := cursorPageSize(in.PageSize, policy)
	out, err := s.deps.Runtime.Fetch(ctx, scope(p, ref), execution.FetchRequest{
		SessionID: in.SessionID,
		CursorID:  in.CursorID,
		PageSize:  pageSize,
		Limits:    limits(policy),
	})
	if err != nil {
		return CursorPage{PageSize: pageSize}, &RuntimeError{Err: err}
	}
	out.CursorID = in.CursorID
	return CursorPage{Result: out, PageSize: pageSize}, nil
}

// CloseCursor is idempotent: closing an unknown or foreign cursor succeeds
// exactly like closing one's own.
func (s *Service) CloseCursor(ctx context.Context, p access.Principal, ref access.ConnRef, sessionID execution.SessionID, cursorID execution.CursorID) error {
	if _, _, err := s.deps.Requirer.ResolveConnection(ctx, p, ref); err != nil {
		return err
	}
	if cursorID == "" {
		return ErrNotFound
	}
	return s.deps.Runtime.CloseCursor(ctx, scope(p, ref), sessionID, cursorID)
}

// cursorPageSize defaults to the configured cursor page size and never
// exceeds the result row ceiling.
func cursorPageSize(requested *int, policy Policy) int {
	pageSize := policy.CursorPageSize
	if requested != nil {
		pageSize = *requested
	}
	if pageSize > policy.MaxRows {
		return policy.MaxRows
	}
	return pageSize
}
