package oracle

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/transaction"
)

func TestOracleTransactionGuards(t *testing.T) {
	d := &oracleDriver{}
	if d.InTransaction() {
		t.Fatal("fresh driver reports an open transaction")
	}
	if err := d.Commit(context.Background()); !errors.Is(err, transaction.ErrNoOpenTransaction) {
		t.Fatalf("Commit with no tx = %v", err)
	}
	if err := d.Rollback(context.Background()); !errors.Is(err, transaction.ErrNoOpenTransaction) {
		t.Fatalf("Rollback with no tx = %v", err)
	}
	if err := d.Savepoint(context.Background(), "sqlwarden_sp_1"); !errors.Is(err, transaction.ErrNoOpenTransaction) {
		t.Fatalf("Savepoint with no tx = %v", err)
	}
	if err := d.RollbackToSavepoint(context.Background(), "sqlwarden_sp_1"); !errors.Is(err, transaction.ErrNoOpenTransaction) {
		t.Fatalf("RollbackToSavepoint with no tx = %v", err)
	}
}
