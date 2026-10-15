package query

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/execution"
)

// TransactionStatus reports the transaction state of the principal's session.
func (s *Service) TransactionStatus(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) (execution.TxStatus, error) {
	if err := s.transactionSession(ctx, p, ref, id); err != nil {
		return execution.TxStatus{}, err
	}
	return s.deps.Runtime.Status(ctx, scope(p, ref), id)
}

// SetTransactionMode switches the session between auto-commit and manual
// transactions. The session is confirmed live before the mode is validated.
func (s *Service) SetTransactionMode(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, mode string) (execution.TxStatus, error) {
	if err := s.transactionSession(ctx, p, ref, id); err != nil {
		return execution.TxStatus{}, err
	}
	sc := scope(p, ref)
	if _, err := s.deps.Runtime.Get(ctx, sc, id); err != nil {
		return execution.TxStatus{}, err
	}
	if mode != string(execution.TxModeAuto) && mode != string(execution.TxModeManual) {
		return execution.TxStatus{}, &ValidationError{Fields: map[string]string{"mode": "Mode must be auto or manual."}}
	}
	return s.deps.Runtime.SetMode(ctx, sc, id, execution.TxMode(mode))
}

// Commit commits the session's open transaction.
func (s *Service) Commit(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) (execution.TxStatus, error) {
	if err := s.transactionSession(ctx, p, ref, id); err != nil {
		return execution.TxStatus{}, err
	}
	return s.deps.Runtime.Commit(ctx, scope(p, ref), id)
}

// Rollback rolls back the session's open transaction. It requires workspace
// membership but no connection permission, so a principal whose access was
// revoked mid-transaction can still release it.
func (s *Service) Rollback(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) (execution.TxStatus, error) {
	if _, _, err := s.deps.Requirer.ConnectionMember(ctx, p, ref); err != nil {
		return execution.TxStatus{}, err
	}
	if id == "" {
		return execution.TxStatus{}, ErrSessionRequired
	}
	return s.deps.Runtime.Rollback(ctx, scope(p, ref), id)
}

func (s *Service) transactionSession(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) error {
	if _, _, err := s.runtimeConnection(ctx, p, ref); err != nil {
		return err
	}
	if id == "" {
		return ErrSessionRequired
	}
	return nil
}
