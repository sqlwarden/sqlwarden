package postgres

import "github.com/sqlwarden/internal/dbengine"

// postgresDriver must implement the engine connection contract.
var _ dbengine.Driver = (*postgresDriver)(nil)

func init() {
	dbengine.Register(dbengine.Registration{
		ID:          "postgres",
		DisplayName: "PostgreSQL",
		Dialect:     dbengine.DialectPostgres,
		New:         func() dbengine.Driver { return &postgresDriver{} },
	})
}
