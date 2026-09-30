package supabase

import (
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
)

// driver is the Supabase engine.Driver implementation: postgres.Driver plus
// filtering of Supabase's own managed schemas out of the schema browser.
type driver struct {
	postgres.Driver
}

var (
	_ engine.Driver           = (*driver)(nil)
	_ engine.TLSCapable       = (*driver)(nil)
	_ engine.SSHTunnelCapable = (*driver)(nil)
)

// managedSchemas are the schemas a Supabase project provisions for its own
// platform features rather than user data. They exist on every Supabase
// Postgres instance regardless of what the user has built, so the schema
// browser excludes them by default the same way plain PostgreSQL excludes
// pg_catalog/information_schema.
var managedSchemas = map[string]bool{
	"auth":               true,
	"storage":            true,
	"realtime":           true,
	"extensions":         true,
	"graphql":            true,
	"graphql_public":     true,
	"supabase_functions": true,
	"pgbouncer":          true,
	"vault":              true,
}

func (d *driver) Dialect() engine.Dialect { return engine.DialectPostgres }

func init() {
	engine.Register(engine.Registration{
		ID:          "supabase",
		DisplayName: "Supabase",
		Dialect:     engine.DialectPostgres,
		New:         func() engine.Driver { return &driver{} },
	})
}
