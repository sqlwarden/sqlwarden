package postgres

import "github.com/sqlwarden/internal/engine"

// postgresDriver must implement the engine connection contract.
var _ engine.Driver = (*postgresDriver)(nil)

func init() {
	engine.Register(engine.Registration{
		ID:          "postgres",
		DisplayName: "PostgreSQL",
		Dialect:     engine.DialectPostgres,
		New:         func() engine.Driver { return &postgresDriver{} },
	})
}
