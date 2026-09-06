// Package cockroachdb implements the CockroachDB engine as a compatible
// extension of postgres.Driver. See doc.go for the extension pattern and the
// specific points of divergence from PostgreSQL.
package cockroachdb

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
)

// driver is the CockroachDB engine.Driver implementation: postgres.Driver
// plus overrides for the points where CockroachDB's SQL surface diverges
// from PostgreSQL (schema kinds, EXPLAIN syntax).
type driver struct {
	postgres.Driver
}

var (
	_ engine.Driver           = (*driver)(nil)
	_ engine.TLSCapable       = (*driver)(nil)
	_ engine.SSHTunnelCapable = (*driver)(nil)
)

func (d *driver) Dialect() engine.Dialect { return engine.DialectCockroachDB }

func init() {
	engine.Register(engine.Registration{
		ID:          "cockroachdb",
		DisplayName: "CockroachDB",
		Dialect:     engine.DialectCockroachDB,
		New:         func() engine.Driver { return &driver{} },
	})
}
