package cockroachdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var _ metadata.SchemaInspector = (*driver)(nil)

// SchemaSpec matches postgres.Driver.SchemaSpec minus materialized_view:
// CockroachDB has no CREATE MATERIALIZED VIEW support.
func (d *driver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "cockroachdb",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
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

	b := build.NewDirectory()
	b.AddScope(root)
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("function")
	b.DeclareKind("sequence")

	if err := postgres.CatalogTables(ctx, db, func(ns, name, kind string) {
		if systemSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), kind, name)
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog tables: %w", err)
	}
	if err := attachRowCounts(ctx, db, func(ns, name string, count int64) {
		if systemSchemas[ns] {
			return
		}
		b.SetRowCount(scope(ns), "table", name, count)
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog row counts: %w", err)
	}
	if err := postgres.CatalogFunctions(ctx, db, func(ns, name string) {
		if systemSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), "function", name)
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog functions: %w", err)
	}
	if err := postgres.CatalogSequences(ctx, db, func(ns, name string) {
		if systemSchemas[ns] {
			return
		}
		b.AddRef(scope(ns), "sequence", name)
	}); err != nil {
		return nil, fmt.Errorf("cockroachdb: catalog sequences: %w", err)
	}

	return b.Build("", "cockroachdb", defaultScope), nil
}

// InspectObjects mirrors postgres.Driver.InspectObjects but drops the
// materialized_view bucket.
func (d *driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	db := d.DB()
	var relRefs, fnRefs, seqRefs []metadata.ObjectRef
	for _, r := range refs {
		switch r.Kind {
		case "table", "view":
			relRefs = append(relRefs, r)
		case "function":
			fnRefs = append(fnRefs, r)
		case "sequence":
			seqRefs = append(seqRefs, r)
		}
	}

	var out []metadata.Object
	if len(relRefs) > 0 {
		objs, err := postgres.RelationalObjects(ctx, db, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(fnRefs) > 0 {
		objs, err := functionObjects(ctx, db, fnRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(seqRefs) > 0 {
		objs, err := postgres.SequenceObjects(ctx, db, seqRefs)
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

// InspectDefinition delegates every kind except function to postgres.Driver.
// function is overridden because functionDefinition (catalog.go) must
// recover the language from pg_get_functiondef's own LANGUAGE clause rather
// than postgres.FunctionDefinition's pg_language join, which always returns
// zero rows on CockroachDB.
func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	if ref.Kind != "function" {
		return d.Driver.InspectDefinition(ctx, ref)
	}
	language, def, err := functionDefinition(ctx, d.DB(), ref)
	if err != nil {
		return nil, err
	}
	return postgres.SourceDescriptor("Definition", language, def), nil
}
