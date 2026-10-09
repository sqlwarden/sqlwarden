package query

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

type HistoryInput struct {
	SQL          string
	Status       string
	ErrorMessage *string
	DurationMS   int64
	RowsAffected int64
}

func supportedHistoryStatus(status string) bool {
	switch status {
	case "ok", "error", "cancelled":
		return true
	default:
		return false
	}
}

// RecordHistory stores an entry for the principal on a connection of a
// workspace they belong to, trimmed to the configured retention.
func (s *Service) RecordHistory(ctx context.Context, p access.Principal, ref access.ConnRef, in HistoryInput) (database.QueryHistoryEntry, error) {
	conn, ws, err := s.deps.Requirer.ConnectionMember(ctx, p, ref)
	if err != nil {
		return database.QueryHistoryEntry{}, err
	}

	fields := map[string]string{}
	if in.SQL == "" {
		fields["sql_text"] = "SQL text is required."
	}
	if !supportedHistoryStatus(in.Status) {
		fields["status"] = "Status must be ok, error, or cancelled."
	}
	if len(fields) != 0 {
		return database.QueryHistoryEntry{}, &ValidationError{Fields: fields}
	}

	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return database.QueryHistoryEntry{}, err
	}
	if policy.HistoryMode != modeBackend {
		return database.QueryHistoryEntry{}, &DisabledError{Message: messageHistoryDisabled}
	}

	return s.deps.Store.InsertQueryHistoryEntry(ctx, database.QueryHistoryEntry{
		ConnectionID: conn.ID,
		AccountID:    s.accountID(p),
		SQL:          in.SQL,
		Status:       in.Status,
		ErrorMessage: in.ErrorMessage,
		DurationMS:   in.DurationMS,
		RowsAffected: in.RowsAffected,
	}, policy.HistoryRetention)
}

func (s *Service) ListHistory(ctx context.Context, p access.Principal, ref access.ConnRef, q ListQuery) (response.Paginated[database.QueryHistoryEntry], error) {
	conn, _, err := s.deps.Requirer.ConnectionMember(ctx, p, ref)
	if err != nil {
		return response.Paginated[database.QueryHistoryEntry]{}, err
	}
	return s.deps.Store.ListQueryHistory(ctx, conn.ID, s.accountID(p), q.Search, q.Page, q.PageSize)
}

func (s *Service) ListWorkspaceHistory(ctx context.Context, p access.Principal, orgID, wsID int64, connectionID *int64, q ListQuery) (response.Paginated[database.QueryHistoryEntry], error) {
	ws, err := s.deps.Requirer.WorkspaceMember(ctx, p, orgID, wsID)
	if err != nil {
		return response.Paginated[database.QueryHistoryEntry]{}, err
	}
	return s.deps.Store.ListQueryHistoryForWorkspace(ctx, ws.ID, s.accountID(p), connectionID, q.Search, q.Page, q.PageSize)
}

// DeleteHistoryEntry removes one of the principal's own entries. Another
// user's entry is reported as ErrNotFound.
func (s *Service) DeleteHistoryEntry(ctx context.Context, p access.Principal, ref access.ConnRef, id int64) error {
	conn, _, err := s.deps.Requirer.ConnectionMember(ctx, p, ref)
	if err != nil {
		return err
	}
	found, err := s.deps.Store.DeleteQueryHistoryEntry(ctx, id, conn.ID, s.accountID(p))
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

func (s *Service) ClearHistory(ctx context.Context, p access.Principal, ref access.ConnRef) error {
	conn, _, err := s.deps.Requirer.ConnectionMember(ctx, p, ref)
	if err != nil {
		return err
	}
	return s.deps.Store.ClearQueryHistoryForConnection(ctx, conn.ID, s.accountID(p))
}
