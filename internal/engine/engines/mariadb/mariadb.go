package mariadb

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/mysql"
)

// driver is the MariaDB engine.Driver implementation: mysql.Driver plus
// native sequence support and MariaDB's JSON-as-LONGTEXT column reporting.
type driver struct {
	mysql.Driver
}

var (
	_ engine.Driver           = (*driver)(nil)
	_ engine.TLSCapable       = (*driver)(nil)
	_ engine.SSHTunnelCapable = (*driver)(nil)
)

func (d *driver) Dialect() engine.Dialect { return engine.DialectMariaDB }

func init() {
	engine.Register(engine.Registration{
		ID:          "mariadb",
		DisplayName: "MariaDB",
		Dialect:     engine.DialectMariaDB,
		New:         func() engine.Driver { return &driver{} },
	})
}
