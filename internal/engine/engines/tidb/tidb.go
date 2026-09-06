package tidb

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/mysql"
)

// driver is the TiDB engine.Driver implementation: mysql.Driver plus native
// sequence support and the catalog kinds TiDB does not implement (function,
// procedure, trigger).
type driver struct {
	mysql.Driver
}

var (
	_ engine.Driver           = (*driver)(nil)
	_ engine.TLSCapable       = (*driver)(nil)
	_ engine.SSHTunnelCapable = (*driver)(nil)
)

// Dialect reports engine.DialectMySQL: TiDB targets the MySQL SQL surface for
// every statement form SQLWarden classifies, parses, or completes, so it
// needs no dialect identity of its own.
func (d *driver) Dialect() engine.Dialect { return engine.DialectMySQL }

func init() {
	engine.Register(engine.Registration{
		ID:          "tidb",
		DisplayName: "TiDB",
		Dialect:     engine.DialectMySQL,
		New:         func() engine.Driver { return &driver{} },
	})
}
