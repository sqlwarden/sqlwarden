package sqlite

import "github.com/sqlwarden/internal/dbengine"

// sqliteDriver must implement the engine connection contract.
var _ dbengine.Driver = (*sqliteDriver)(nil)

func init() {
	dbengine.Register(dbengine.Registration{
		ID:          "sqlite",
		DisplayName: "SQLite",
		Dialect:     dbengine.DialectSQLite,
		New:         func() dbengine.Driver { return &sqliteDriver{} },
	})
}
