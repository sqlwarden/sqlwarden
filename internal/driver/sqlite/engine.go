package sqlite

import (
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	dbengine.Register(dbengine.Registration{
		ID:          "sqlite",
		DisplayName: "SQLite",
		Dialect:     driver.DialectSQLite,
		NewDriver:   func() driver.Driver { return &sqliteDriver{} },
	})
}
