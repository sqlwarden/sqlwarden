package oracle

import (
	"context"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/engine/transaction"
)

var _ transaction.SavepointController = (*oracleDriver)(nil)

var errAlreadyInTransaction = errors.New("transaction already open")

func (d *oracleDriver) BeginTx(ctx context.Context) error {
	if d.currentTx != nil {
		return fmt.Errorf("oracle: begin tx: %w", errAlreadyInTransaction)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("oracle: begin tx: %w", err)
	}
	d.currentTx = tx
	return nil
}

func (d *oracleDriver) Commit(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Commit()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("oracle: commit: %w", err)
	}
	return nil
}

func (d *oracleDriver) Rollback(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Rollback()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("oracle: rollback: %w", err)
	}
	return nil
}

func (d *oracleDriver) InTransaction() bool { return d.currentTx != nil }

func (d *oracleDriver) Savepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	// name always comes from transaction.NewSavepointName, never user input.
	// codeql[go/sql-injection]
	if _, err := d.currentTx.ExecContext(ctx, "SAVEPOINT "+oracleQuoteIdent(name)); err != nil {
		return fmt.Errorf("oracle: savepoint: %w", err)
	}
	return nil
}

func (d *oracleDriver) RollbackToSavepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	// codeql[go/sql-injection]
	if _, err := d.currentTx.ExecContext(ctx, "ROLLBACK TO "+oracleQuoteIdent(name)); err != nil {
		return fmt.Errorf("oracle: rollback to savepoint: %w", err)
	}
	return nil
}
