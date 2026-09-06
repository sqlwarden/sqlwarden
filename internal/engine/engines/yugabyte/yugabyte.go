// Package yugabyte implements the YugabyteDB engine as a compatible
// extension of postgres.Driver. See doc.go for the extension pattern.
package yugabyte

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
)

// driver is the YugabyteDB engine.Driver implementation: postgres.Driver with
// only its registered identity distinct. YugabyteDB's YSQL API is a
// PostgreSQL-compatible query layer built on the PostgreSQL upstream source,
// so it accepts the same SQL grammar, EXPLAIN output, and
// pg_catalog/information_schema shape the embedded implementation already
// queries.
type driver struct {
	postgres.Driver
}

var (
	_ engine.Driver           = (*driver)(nil)
	_ engine.TLSCapable       = (*driver)(nil)
	_ engine.SSHTunnelCapable = (*driver)(nil)
)

func (d *driver) Dialect() engine.Dialect { return engine.DialectPostgres }

func init() {
	engine.Register(engine.Registration{
		ID:          "yugabyte",
		DisplayName: "YugabyteDB",
		Dialect:     engine.DialectPostgres,
		New:         func() engine.Driver { return &driver{} },
	})
}
