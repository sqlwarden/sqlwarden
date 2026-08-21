package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/sqlwarden/internal/engine/transaction"
)

var errAlreadyInTransaction = errors.New("transaction already open")

var _ transaction.SavepointController = (*postgresDriver)(nil)

func (d *postgresDriver) BeginTx(ctx context.Context) error {
	if d.currentTx != nil {
		return fmt.Errorf("postgres: begin tx: %w", errAlreadyInTransaction)
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}
	d.currentTx = tx
	return nil
}

func (d *postgresDriver) Commit(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Commit()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

func (d *postgresDriver) Rollback(ctx context.Context) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	err := d.currentTx.Rollback()
	d.currentTx = nil
	if err != nil {
		return fmt.Errorf("postgres: rollback: %w", err)
	}
	return nil
}

func (d *postgresDriver) InTransaction() bool {
	return d.currentTx != nil
}

func (d *postgresDriver) Savepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	// name always comes from transaction.NewSavepointName, never user input.
	if _, err := d.currentTx.ExecContext(ctx, "SAVEPOINT "+pgQuoteIdent(name)); err != nil {
		return fmt.Errorf("postgres: savepoint: %w", err)
	}
	return nil
}

func (d *postgresDriver) RollbackToSavepoint(ctx context.Context, name string) error {
	if d.currentTx == nil {
		return transaction.ErrNoOpenTransaction
	}
	if _, err := d.currentTx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+pgQuoteIdent(name)); err != nil {
		return fmt.Errorf("postgres: rollback to savepoint: %w", err)
	}
	return nil
}
