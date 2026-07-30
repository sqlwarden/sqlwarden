package sqlite

import "github.com/sqlwarden/internal/engine"

// sqliteDriver must implement the engine connection contract.
var _ engine.Driver = (*sqliteDriver)(nil)

func init() {
	engine.Register(engine.Registration{
		ID:          "sqlite",
		DisplayName: "SQLite",
		Dialect:     engine.DialectSQLite,
		New:         func() engine.Driver { return &sqliteDriver{} },
	})
}
