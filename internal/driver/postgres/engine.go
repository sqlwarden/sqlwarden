package postgres

import (
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	dbengine.Register(dbengine.Registration{
		ID:          "postgres",
		DisplayName: "PostgreSQL",
		Dialect:     driver.DialectPostgres,
		NewDriver:   func() driver.Driver { return &postgresDriver{} },
	})
}
