package execution

import (
	"context"

	"github.com/sqlwarden/internal/connection"
)

func (r *LocalRuntime) Status(ctx context.Context, scope Scope, id SessionID) (TxStatus, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return TxStatus{}, err
	}
	return txSnapshot(sess), nil
}

func (r *LocalRuntime) SetMode(ctx context.Context, scope Scope, id SessionID, mode TxMode) (TxStatus, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return TxStatus{}, err
	}
	if mode != TxModeAuto && mode != TxModeManual {
		return TxStatus{}, ErrInvalidTxMode
	}
	if err := sess.SetTransactionMode(ctx, connection.TxMode(mode)); err != nil {
		return TxStatus{}, mapError(err, opTransaction)
	}
	return txSnapshot(sess), nil
}

func (r *LocalRuntime) Commit(ctx context.Context, scope Scope, id SessionID) (TxStatus, error) {
	sess, err := r.txSession(scope, id)
	if err != nil {
		return TxStatus{}, err
	}
	// Drivers differ in whether Commit honors ctx, so a cancelled caller is
	// refused up front; the transaction stays open and the caller must treat
	// the outcome as unknown.
	if err := ctx.Err(); err != nil {
		return TxStatus{}, mapError(err, opCommit)
	}
	if err := sess.CommitTransaction(ctx); err != nil {
		return TxStatus{}, mapError(err, opCommit)
	}
	return txSnapshot(sess), nil
}

func (r *LocalRuntime) Rollback(ctx context.Context, scope Scope, id SessionID) (TxStatus, error) {
	sess, err := r.txSession(scope, id)
	if err != nil {
		return TxStatus{}, err
	}
	if err := sess.RollbackTransaction(ctx); err != nil {
		return TxStatus{}, mapError(err, opTransaction)
	}
	return txSnapshot(sess), nil
}

// txSession resolves a session for commit or rollback. A missing session took
// its open transaction with it, and a session owned by another scope is
// reported identically so callers cannot probe for other principals' sessions.
func (r *LocalRuntime) txSession(scope Scope, id SessionID) (*connection.Session, error) {
	sess, ok := r.manager.Get(string(id))
	if !ok || !ownedBy(sess, scope) {
		return nil, &Failure{Code: FailureTransactionLost, Err: ErrTransactionLost}
	}
	return sess, nil
}

func txSnapshot(sess *connection.Session) TxStatus {
	st := txStatusOf(sess)
	if st.Statements == nil {
		st.Statements = []string{}
	}
	return st
}
