package mysql

import "github.com/sqlwarden/internal/dbengine"

// mysqlDriver must implement the engine connection contract.
var _ dbengine.Driver = (*mysqlDriver)(nil)

func init() {
	dbengine.Register(dbengine.Registration{
		ID:          "mysql",
		DisplayName: "MySQL",
		Dialect:     dbengine.DialectMySQL,
		New:         func() dbengine.Driver { return &mysqlDriver{} },
	})
}
