package query

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/execution"
)

var (
	ErrStatementsUnsupported = errors.New("query: statement generation is not supported by this driver")
	ErrStatementUnsupported  = errors.New("query: statement is not supported for this object")
)

// StatementError reports a statement the generator could not build. Its
// message comes from the generator and names no row values.
type StatementError struct {
	Err error
}

func (e *StatementError) Error() string { return e.Err.Error() }
func (e *StatementError) Unwrap() error { return e.Err }

// GenerateStatement renders a statement for one object, described from the
// cache or the session.
func (s *Service) GenerateStatement(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, operation statement.Operation, object metadata.ObjectRef) (string, error) {
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return "", err
	}
	generator, ok := s.deps.Engines.StatementGenerator(req.conn.Driver)
	if !ok {
		return "", ErrStatementsUnsupported
	}
	if !generator.StatementSpec().Supports(object.Kind, operation) {
		return "", ErrStatementUnsupported
	}
	objects, err := s.schemaObjects(ctx, req, []metadata.ObjectRef{object})
	if err != nil {
		return "", err
	}
	if len(objects) != 1 || objects[0].Ref != object {
		return "", ErrSchemaObjectNotFound
	}
	generated, err := generator.Generate(statement.Request{Operation: operation, Object: objects[0]})
	if err != nil {
		return "", &StatementError{Err: err}
	}
	return generated, nil
}
