package mysql

import (
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/driver"
)

func init() {
	dbengine.Register(dbengine.Registration{
		ID:          "mysql",
		DisplayName: "MySQL",
		Dialect:     driver.DialectMySQL,
		NewDriver:   func() driver.Driver { return &mysqlDriver{} },
	})
}
