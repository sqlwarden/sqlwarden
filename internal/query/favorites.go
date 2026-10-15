package query

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
)

type FavoriteInput struct {
	Name         string
	SQL          string
	ConnectionID *int64
}

func validateFavorite(name, sql string) error {
	fields := map[string]string{}
	if name == "" {
		fields["name"] = "Name is required."
	}
	if sql == "" {
		fields["sql_text"] = "SQL text is required."
	}
	if len(fields) != 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func (s *Service) validateFavoriteConnection(ctx context.Context, p access.Principal, orgID, wsID int64, connectionID *int64) error {
	if connectionID == nil {
		return nil
	}
	_, _, err := s.deps.Requirer.ResolveConnection(ctx, p, access.ConnRef{OrgID: orgID, WorkspaceID: wsID, ConnectionID: *connectionID})
	if errors.Is(err, access.ErrNotFound) {
		return &ValidationError{Fields: map[string]string{"connection_id": "Connection was not found in this workspace."}}
	}
	return err
}

func (s *Service) ListFavorites(ctx context.Context, p access.Principal, orgID, wsID int64, search string) ([]database.QueryFavorite, error) {
	ws, err := s.deps.Requirer.WorkspaceMember(ctx, p, orgID, wsID)
	if err != nil {
		return nil, err
	}
	return s.deps.Store.ListQueryFavorites(ctx, ws.ID, s.accountID(p), search)
}

func (s *Service) CreateFavorite(ctx context.Context, p access.Principal, orgID, wsID int64, in FavoriteInput) (database.QueryFavorite, error) {
	ws, err := s.deps.Requirer.WorkspaceMember(ctx, p, orgID, wsID)
	if err != nil {
		return database.QueryFavorite{}, err
	}
	if err := validateFavorite(in.Name, in.SQL); err != nil {
		return database.QueryFavorite{}, err
	}
	if err := s.validateFavoriteConnection(ctx, p, orgID, ws.ID, in.ConnectionID); err != nil {
		return database.QueryFavorite{}, err
	}
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return database.QueryFavorite{}, err
	}
	if policy.FavoritesMode != modeBackend {
		return database.QueryFavorite{}, &DisabledError{Message: messageFavoritesDisabled}
	}
	return s.deps.Store.CreateQueryFavorite(ctx, database.QueryFavorite{
		WorkspaceID:  ws.ID,
		AccountID:    s.accountID(p),
		ConnectionID: in.ConnectionID,
		Name:         in.Name,
		SQL:          in.SQL,
	})
}

// UpdateFavorite changes one of the principal's own favorites. Another user's
// favorite is reported as ErrNotFound.
func (s *Service) UpdateFavorite(ctx context.Context, p access.Principal, orgID, wsID, id int64, in FavoriteInput) (database.QueryFavorite, error) {
	ws, err := s.deps.Requirer.WorkspaceMember(ctx, p, orgID, wsID)
	if err != nil {
		return database.QueryFavorite{}, err
	}
	if err := validateFavorite(in.Name, in.SQL); err != nil {
		return database.QueryFavorite{}, err
	}
	if err := s.validateFavoriteConnection(ctx, p, orgID, ws.ID, in.ConnectionID); err != nil {
		return database.QueryFavorite{}, err
	}
	updated, found, err := s.deps.Store.UpdateQueryFavorite(ctx, id, ws.ID, s.accountID(p), in.Name, in.SQL)
	if err != nil {
		return database.QueryFavorite{}, err
	}
	if !found {
		return database.QueryFavorite{}, ErrNotFound
	}
	return updated, nil
}

func (s *Service) DeleteFavorite(ctx context.Context, p access.Principal, orgID, wsID, id int64) error {
	ws, err := s.deps.Requirer.WorkspaceMember(ctx, p, orgID, wsID)
	if err != nil {
		return err
	}
	found, err := s.deps.Store.DeleteQueryFavorite(ctx, id, ws.ID, s.accountID(p))
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	return nil
}
