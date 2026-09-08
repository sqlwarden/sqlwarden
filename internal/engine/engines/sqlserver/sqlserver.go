package sqlserver

import "github.com/sqlwarden/internal/engine"

var _ engine.Driver = (*Driver)(nil)

func init() {
	engine.Register(engine.Registration{
		ID:          "sqlserver",
		DisplayName: "SQL Server",
		Dialect:     engine.DialectSQLServer,
		New:         func() engine.Driver { return &Driver{} },
	})
}
