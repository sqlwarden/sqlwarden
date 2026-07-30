package mysql

import "github.com/sqlwarden/internal/engine"

// mysqlDriver must implement the engine connection contract.
var _ engine.Driver = (*mysqlDriver)(nil)

func init() {
	engine.Register(engine.Registration{
		ID:          "mysql",
		DisplayName: "MySQL",
		Dialect:     engine.DialectMySQL,
		New:         func() engine.Driver { return &mysqlDriver{} },
	})
}
