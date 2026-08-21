// Package transaction defines the optional engine capability for
// manual-commit transaction control, following the same optional-capability
// pattern as internal/engine/cursor and internal/engine/ddl.
package transaction

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoOpenTransaction is returned by Commit/Rollback/Savepoint operations
// when no transaction is open.
var ErrNoOpenTransaction = errors.New("no open transaction")

// Controller is implemented by drivers that support manual-commit
// transactions. A driver without SavepointController still supports manual
// mode; it just loses per-statement failure isolation.
type Controller interface {
	BeginTx(ctx context.Context) error
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	InTransaction() bool
}

// SavepointController is implemented by drivers that additionally support
// SAVEPOINT / ROLLBACK TO SAVEPOINT for per-statement failure isolation
// inside an open transaction. All three current engines (postgres, mysql,
// sqlite) implement it.
type SavepointController interface {
	Controller
	Savepoint(ctx context.Context, name string) error
	RollbackToSavepoint(ctx context.Context, name string) error
}

// NewSavepointName returns a generated, inert savepoint identifier. Callers
// must never derive a savepoint name from user input.
func NewSavepointName(n int) string {
	return fmt.Sprintf("sqlwarden_sp_%d", n)
}
