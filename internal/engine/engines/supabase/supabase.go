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

	b := build.NewDirectory()
	b.AddScope(root)
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("materialized_view")
	b.DeclareKind("function")
	b.DeclareKind("sequence")

	if err := postgres.CatalogTables(ctx, db, func(ns, name, kind string) {
		if managedSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), kind, name)
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog tables: %w", err)
	}
	if err := postgres.CatalogMaterializedViews(ctx, db, func(ns, name string) {
		if managedSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), "materialized_view", name)
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog matviews: %w", err)
	}
	if err := postgres.AttachRowCounts(ctx, db, func(ns, kind, name string, count int64) {
		if managedSchemas[ns] {
			return
		}
		b.SetRowCount(scope(ns), kind, name, count)
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog row counts: %w", err)
	}
	if err := postgres.CatalogFunctions(ctx, db, func(ns, name string) {
		if managedSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), "function", name)
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog functions: %w", err)
	}
	if err := postgres.CatalogSequences(ctx, db, func(ns, name string) {
		if managedSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), "sequence", name)
	}); err != nil {
		return nil, fmt.Errorf("supabase: catalog sequences: %w", err)
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
