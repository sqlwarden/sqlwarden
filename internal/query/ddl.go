package query

import (
	"context"
	"errors"
	"log/slog"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/execution"
)

const (
	SchemaEditInvalid = "invalid_schema_edit"
	SchemaEditFailed  = "schema_edit_failed"
)

// SchemaEditError reports a structured DDL request the spec or the target
// rejected. Code is SchemaEditInvalid or SchemaEditFailed.
type SchemaEditError struct {
	Code string
	Err  error
}

func (e *SchemaEditError) Error() string { return e.Err.Error() }
func (e *SchemaEditError) Unwrap() error { return e.Err }

// DDLResult reports an applied DDL request. RefreshFailed means the edit
// committed but the affected listings could not be reloaded, so the cache
// under the edit was dropped instead.
type DDLResult struct {
	Transaction   execution.TxStatus
	Persistent    bool
	Listings      []Listing
	RefreshFailed bool
}

// ObjectDDL applies a structured DDL request on the principal's session and
// reloads the navigator listings under the edited scope.
func (s *Service) ObjectDDL(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, input ddl.Request) (DDLResult, error) {
	conn, _, err := s.deps.Requirer.Connection(ctx, p, ref, access.PermConnExecute, access.PermConnDDL)
	if err != nil {
		return DDLResult{}, err
	}
	if id == "" {
		return DDLResult{}, ErrSessionRequired
	}
	req := &schemaRequest{conn: conn, scope: scope(p, ref), requested: id}
	caps, err := s.deps.Runtime.Capabilities(ctx, req.scope, id)
	if err != nil {
		return DDLResult{}, err
	}
	req.session = &schemaSession{id: id, caps: caps}
	if caps.DDL == nil {
		return DDLResult{}, execution.ErrDDLUnsupported
	}
	if err := ddl.Validate(input, *caps.DDL); err != nil {
		return DDLResult{}, &SchemaEditError{Code: SchemaEditInvalid, Err: err}
	}
	tx, err := s.deps.Runtime.ApplyDDL(ctx, req.scope, id, input)
	if err != nil {
		var target *execution.TargetError
		switch {
		case errors.Is(err, execution.ErrDDLUnsupported):
			return DDLResult{}, err
		case errors.Is(err, execution.ErrInvalidDDL):
			return DDLResult{}, &SchemaEditError{Code: SchemaEditInvalid, Err: err}
		case errors.As(err, &target) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
			return DDLResult{}, &SchemaEditError{Code: SchemaEditFailed, Err: target}
		}
		return DDLResult{}, err
	}
	s.deps.Logger.InfoContext(ctx, "DDL applied",
		slog.String("session_id", string(id)),
		slog.Int64("connection_id", conn.ID),
		slog.String("operation", string(input.Operation)),
	)

	nav, err := s.navigatorConnection(ctx, conn)
	if err != nil {
		return DDLResult{}, err
	}
	out := DDLResult{Transaction: tx, Persistent: nav.Persistent}
	tree, ok := s.navigatorTree(conn.Driver)
	if !ok {
		return out, nil
	}
	root := input.Scope
	if input.Ref != nil {
		root = input.Ref.Path()
	}
	listings, refreshErr := s.deps.Navigator.Refresh(ctx, nav, tree, s.navigatorLive(req), root)
	if refreshErr != nil {
		s.deps.Logger.WarnContext(ctx, "schema edit listing refresh failed",
			slog.Int64("connection_id", conn.ID),
			slog.String("operation", string(input.Operation)),
			slog.Any("error", refreshErr),
		)
		if invalidateErr := s.deps.Navigator.Invalidate(ctx, nav, root); invalidateErr != nil {
			s.deps.Logger.WarnContext(ctx, "schema edit cache invalidation failed",
				slog.Int64("connection_id", conn.ID),
				slog.Any("error", invalidateErr),
			)
		}
		out.RefreshFailed = true
		return out, nil
	}
	out.Listings = listings
	return out, nil
}
