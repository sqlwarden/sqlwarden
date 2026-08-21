package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/engine/transaction"
)

var _ transaction.SavepointController = (*sqliteDriver)(nil)

var errAlreadyInTransaction = errors.New("transaction already open")

func (d *sqliteDriver) BeginTx(ctx context.Context) error {
	if d.currentTx != nil {
		return fmt.Errorf("sqlite: begin tx: %w", errAlreadyInTransaction)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin tx: %w", err)
	}
	d.currentTx = tx
	return nil
}

func (d *sqliteDriver) Commit(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Commit()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("sqlite: commit: %w", err)
	}
	return nil
}

func (d *sqliteDriver) Rollback(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Rollback()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("sqlite: rollback: %w", err)
	}
	return nil
}

func (d *sqliteDriver) InTransaction() bool {
	return d.currentTx != nil
}

func (d *sqliteDriver) Savepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	// name always comes from transaction.NewSavepointName, never user input.
	if _, err := d.currentTx.ExecContext(ctx, `SAVEPOINT "`+name+`"`); err != nil {
		return fmt.Errorf("sqlite: savepoint: %w", err)
	}
	return nil
}

func (d *sqliteDriver) RollbackToSavepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	if _, err := d.currentTx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT "`+name+`"`); err != nil {
		return fmt.Errorf("sqlite: rollback to savepoint: %w", err)
	}
	return nil
}
