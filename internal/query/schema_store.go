package query

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/schema"
)

// SchemaStore reads the per-connection settings the schema navigator applies
// at read time.
type SchemaStore interface {
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
	SchemaSnapshotsEnabled(ctx context.Context, connectionID int64) (bool, error)
}

// SchemaEngines resolves the static schema capabilities of a driver.
type SchemaEngines interface {
	Describe(driver string) (engine.CapabilitySet, bool)
	StatementGenerator(driver string) (statement.Generator, bool)
}

// EngineRegistry resolves schema capabilities from the registered engines.
type EngineRegistry struct{}

var _ SchemaEngines = EngineRegistry{}

func (EngineRegistry) Describe(driver string) (engine.CapabilitySet, bool) {
	return engine.Describe(driver)
}

func (EngineRegistry) StatementGenerator(driver string) (statement.Generator, bool) {
	return engine.StatementGenerator(driver)
}

// Listing is a navigator folder listing.
type Listing = schema.Listing

// navigatorConnection projects the stored connection settings onto the
// navigator. A connection that disappeared after authorization reads as not
// found.
func (s *Service) navigatorConnection(ctx context.Context, conn access.Connection) (schema.Connection, error) {
	stored, found, err := s.deps.SchemaStore.GetConnection(ctx, conn.ID)
	if err != nil {
		return schema.Connection{}, err
	}
	if !found {
		return schema.Connection{}, ErrNotFound
	}
	persistent, err := s.deps.SchemaStore.SchemaSnapshotsEnabled(ctx, conn.ID)
	if err != nil {
		return schema.Connection{}, err
	}
	return schema.Connection{
		ID:               stored.ID,
		DefaultScope:     stored.DefaultScope,
		ShowSystem:       stored.ShowSystemSchemas,
		ShowAllDatabases: stored.ShowAllDatabases,
		Persistent:       persistent,
	}, nil
}
