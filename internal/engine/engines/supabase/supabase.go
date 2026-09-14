package supabase

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
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

// InspectDirectory mirrors postgres.Driver.InspectDirectory, composing the
// same exported catalog.go functions, but drops any ref whose schema is
// Supabase-managed before it reaches the builder.
func (d *driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	db := d.DB()
	var database string
	var currentSchema sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&database, &currentSchema); err != nil {
		return nil, fmt.Errorf("supabase: directory database context: %w", err)
	}
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	defaultScope := root
	if currentSchema.Valid && !managedSchemas[currentSchema.String] {
		defaultScope = root.Child(metadata.ScopeSegment{Kind: "schema", Name: currentSchema.String})
	}
	if d.DefaultScope() != "" {
		defaultScope = d.DefaultScope()
	}
	if opts.Root != "" {
		defaultScope = opts.Root
	}
	scope := func(namespace string) metadata.ScopePath {
		return root.Child(metadata.ScopeSegment{Kind: "schema", Name: namespace})
	}

	// A schema-level opts.Root narrows the result to that one schema and makes
	// it the sole root node, matching the contract that a requested scope's
	// detail comes back as Roots[0] rather than nested under the database.
	// An empty or database-level opts.Root returns the full database tree.
	onlySchema := ""
	if opts.Root != "" && opts.Root != root {
		onlySchema = opts.Root.Name("schema")
	}
	included := func(namespace string) bool {
		return !managedSchemas[namespace] && (onlySchema == "" || namespace == onlySchema)
	}

	b := build.NewDirectory()
	if onlySchema == "" {
		b.AddScope(root)
	}
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("materialized_view")
	b.DeclareKind("function")
	b.DeclareKind("sequence")
	b.DeclareKind("procedure")
	b.DeclareKind("trigger")
	b.DeclareKind("type")
	b.DeclareKind("domain")
	b.DeclareKind("foreign_table")

	if err := postgres.CatalogTables(ctx, db, onlySchema, func(ns, name, kind string) {
		if included(ns) {
			b.AddRef(scope(ns), kind, name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog tables: %w", err)
	}
	if err := postgres.CatalogMaterializedViews(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "materialized_view", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog matviews: %w", err)
	}
	if err := postgres.AttachRowCounts(ctx, db, onlySchema, func(ns, kind, name string, count int64) {
		if included(ns) {
			b.SetRowCount(scope(ns), kind, name, count)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog row counts: %w", err)
	}
	if err := postgres.CatalogFunctions(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "function", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog functions: %w", err)
	}
	if err := postgres.CatalogSequences(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "sequence", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog sequences: %w", err)
	}
	if err := postgres.CatalogProcedures(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "procedure", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog procedures: %w", err)
	}
	if err := postgres.CatalogTriggers(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "trigger", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog triggers: %w", err)
	}
	if err := postgres.CatalogTypes(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "type", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog types: %w", err)
	}
	if err := postgres.CatalogDomains(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "domain", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog domains: %w", err)
	}
	if err := postgres.CatalogForeignTables(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "foreign_table", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog foreign tables: %w", err)
	}

	return b.Build("", "supabase", defaultScope), nil
}

// DiscoverScopes delegates to the embedded implementation and filters out
// Supabase-managed schemas from the result.
func (d *driver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	discovery, err := d.Driver.DiscoverScopes(ctx, request)
	if err != nil {
		return nil, err
	}
	filtered := discovery.Scopes[:0]
	for _, scope := range discovery.Scopes {
		if managedSchemas[scope.Name("schema")] {
			continue
		}
		filtered = append(filtered, scope)
	}
	discovery.Scopes = filtered
	return discovery, nil
}

func init() {
	engine.Register(engine.Registration{
		ID:          "supabase",
		DisplayName: "Supabase",
		Dialect:     engine.DialectPostgres,
		New:         func() engine.Driver { return &driver{} },
	})
}
