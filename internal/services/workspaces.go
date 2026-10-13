package services

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/uptrace/bun"
)

// workspaceStore adapts the metadata database and the enforcer's seeding to
// the catalog workspace store.
type workspaceStore struct {
	*database.DB
	enforcer *access.Enforcer
}

func (s workspaceStore) CreateOwnedWorkspace(ctx context.Context, orgID, creatorAccountID int64, name, description string) (database.Workspace, error) {
	if s.enforcer == nil {
		return database.Workspace{}, errors.New("services: enforcer is required to seed workspace roles")
	}
	var ws database.Workspace
	err := s.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		ws, err = s.InsertWorkspaceWithExecutor(ctx, tx, &orgID, "org", orgID, name, description)
		if err != nil {
			return err
		}
		return s.enforcer.SeedWorkspaceWithExecutor(ctx, tx, orgID, ws.ID, creatorAccountID)
	})
	if err != nil {
		return database.Workspace{}, err
	}
	return ws, nil
}
