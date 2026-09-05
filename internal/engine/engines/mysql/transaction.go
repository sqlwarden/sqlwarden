package mysql

import (
	"context"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/engine/transaction"
)

var _ transaction.SavepointController = (*Driver)(nil)

var errAlreadyInTransaction = errors.New("transaction already open")

func (d *Driver) BeginTx(ctx context.Context) error {
	if d.currentTx != nil {
		return fmt.Errorf("mysql: begin tx: %w", errAlreadyInTransaction)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("mysql: begin tx: %w", err)
	}
	d.currentTx = tx
	return nil
}

func (d *Driver) Commit(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Commit()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("mysql: commit: %w", err)
	}
	return nil
}

func (d *Driver) Rollback(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Rollback()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("mysql: rollback: %w", err)
	}
	return nil
}

func (d *Driver) InTransaction() bool {
	return d.currentTx != nil
}

func (d *Driver) Savepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	// name always comes from transaction.NewSavepointName, never user input.
	if _, err := d.currentTx.ExecContext(ctx, "SAVEPOINT `"+name+"`"); err != nil {
		return fmt.Errorf("mysql: savepoint: %w", err)
	}
	return nil
}

func (d *Driver) RollbackToSavepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	if _, err := d.currentTx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT `"+name+"`"); err != nil {
		return fmt.Errorf("mysql: rollback to savepoint: %w", err)
	}
	return nil
}
