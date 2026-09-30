package cockroachdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var _ metadata.DirectoryInspector = (*driver)(nil)

// SchemaSpec matches postgres.Driver.SchemaSpec minus materialized_view:
// CockroachDB has no CREATE MATERIALIZED VIEW support.
func (d *driver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "cockroachdb",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
		},
	}
}

// InspectDirectory mirrors postgres.Driver.InspectDirectory but omits
// CatalogMaterializedViews and the materialized_view row-count bucket.
func (d *driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	db := d.DB()
	var database string
	var currentSchema sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&database, &currentSchema); err != nil {
		return nil, fmt.Errorf("cockroachdb: directory database context: %w", err)
	}
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	defaultScope := root
	if currentSchema.Valid && !systemSchemas[currentSchema.String] {
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
		return !systemSchemas[namespace] && (onlySchema == "" || namespace == onlySchema)
	}

	b := build.NewDirectory()
	if onlySchema == "" {
		b.AddScope(root)
	}
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("function")
	b.DeclareKind("sequence")

	if err := postgres.CatalogTables(ctx, db, onlySchema, func(ns, name, kind string) {
		if included(ns) {
			b.AddRef(scope(ns), kind, name)
		}
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog tables: %w", err)
	}
	if err := attachRowCounts(ctx, db, onlySchema, func(ns, name string, count int64) {
		if included(ns) {
			b.SetRowCount(scope(ns), "table", name, count)
		}
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog row counts: %w", err)
	}
	if err := postgres.CatalogFunctions(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "function", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog functions: %w", err)
	}
	if err := postgres.CatalogSequences(ctx, db, onlySchema, func(ns, name string) {
		if included(ns) {
			b.AddRef(scope(ns), "sequence", name)
		}
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog sequences: %w", err)
	}

	return b.Build("", "cockroachdb", defaultScope), nil
}

// InspectObjects uses postgres detail for every kind except function and
// procedure, whose signature and language CockroachDB only exposes through
// pg_get_functiondef.
func (d *driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	return d.InspectObjectsWith(ctx, refs, inspectObjectsIn)
}

func inspectObjectsIn(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	routines := map[string][]metadata.ObjectRef{}
	var rest []metadata.ObjectRef
	for _, r := range refs {
		if _, ok := routineKinds[r.Kind]; ok {
			routines[r.Kind] = append(routines[r.Kind], r)
		} else {
			rest = append(rest, r)
		}
	}
	out, err := postgres.InspectObjectsIn(ctx, db, rest)
	if err != nil {
		return nil, err
	}
	for kind, kindRefs := range routines {
		objs, err := functionObjects(ctx, db, kind, kindRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// DiscoverScopes delegates to postgres.Driver.DiscoverScopes and then filters
// out system schemas: connection-time scope discovery runs before
// InspectDirectory's own filtering, so without this override
// crdb_internal/pg_extension would leak into the schema-tree UI.
func (d *driver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	discovery, err := d.Driver.DiscoverScopes(ctx, request)
	if err != nil {
		return nil, err
	}
	filtered := discovery.Scopes[:0]
	for _, scope := range discovery.Scopes {
		if systemSchemas[scope.Name("schema")] {
			continue
		}
		filtered = append(filtered, scope)
	}
	discovery.Scopes = filtered
	return discovery, nil
}

// InspectDefinition delegates every kind except function and procedure to
// postgres.Driver. Those are overridden because functionDefinition (catalog.go) must
// recover the language from pg_get_functiondef's own LANGUAGE clause rather
// than postgres.FunctionDefinition's pg_language join, which always returns
// zero rows on CockroachDB.
func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	if _, ok := routineKinds[ref.Kind]; !ok {
		return d.Driver.InspectDefinition(ctx, ref)
	}
	q, err := d.Querier(ctx, ref.Scope.Name("database"))
	if err != nil {
		return nil, err
	}
	language, def, err := functionDefinition(ctx, q, ref)
	if err != nil {
		return nil, err
	}
	return postgres.SourceDescriptor("Definition", language, def), nil
}
