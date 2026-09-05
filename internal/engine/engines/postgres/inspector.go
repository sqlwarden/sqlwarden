package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var _ metadata.SchemaInspector = (*Driver)(nil)
var _ metadata.ScopeDiscoverer = (*Driver)(nil)
var _ metadata.DefinitionInspector = (*Driver)(nil)

func (d *Driver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "postgres",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "materialized_view", Label: "Materialized View", PluralLabel: "Materialized Views", Order: 3, Relational: true, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

// InspectDirectory composes CatalogTables, CatalogMaterializedViews,
// AttachRowCounts, CatalogFunctions, and CatalogSequences from catalog.go. A
// compatible engine that needs a different combination overrides this method
// entirely and calls the same exported functions in whatever shape it needs.
func (d *Driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	var database string
	var currentSchema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&database, &currentSchema); err != nil {
		return nil, fmt.Errorf("postgres: directory database context: %w", err)
	}
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	defaultScope := root
	if currentSchema.Valid {
		defaultScope = root.Child(metadata.ScopeSegment{Kind: "schema", Name: currentSchema.String})
	}
	if d.defaultScope != "" {
		defaultScope = d.defaultScope
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

	if err := CatalogTables(ctx, d.db, func(ns, name, kind string) { b.AddRef(scope(ns), kind, name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog tables: %w", err)
	}
	if err := CatalogMaterializedViews(ctx, d.db, func(ns, name string) { b.AddRef(scope(ns), "materialized_view", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog matviews: %w", err)
	}
	if err := AttachRowCounts(ctx, d.db, func(ns, kind, name string, count int64) { b.SetRowCount(scope(ns), kind, name, count) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog row counts: %w", err)
	}
	if err := CatalogFunctions(ctx, d.db, func(ns, name string) { b.AddRef(scope(ns), "function", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog functions: %w", err)
	}
	if err := CatalogSequences(ctx, d.db, func(ns, name string) { b.AddRef(scope(ns), "sequence", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog sequences: %w", err)
	}

	return b.Build("", "postgres", defaultScope), nil
}

func (d *Driver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	var currentDatabase string
	var currentSchema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&currentDatabase, &currentSchema); err != nil {
		return nil, fmt.Errorf("postgres: discover current scope: %w", err)
	}
	currentRoot := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: currentDatabase})
	current := currentRoot
	if currentSchema.Valid {
		current = current.Child(metadata.ScopeSegment{Kind: "schema", Name: currentSchema.String})
	}
	result := &metadata.ScopeDiscovery{Current: current, Scopes: []metadata.ScopePath{}}
	parentDatabase := request.Parent.Name("database")
	if parentDatabase != "" {
		if parentDatabase != currentDatabase {
			return result, nil
		}
		rows, err := d.db.QueryContext(ctx, `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
  AND schema_name NOT LIKE 'pg_toast%'
  AND schema_name NOT LIKE 'pg_temp_%'
ORDER BY schema_name`)
		if err != nil {
			return nil, fmt.Errorf("postgres: discover schemas: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			result.Scopes = append(result.Scopes, request.Parent.With("schema", name))
		}
		return result, rows.Err()
	}
	rows, err := d.db.QueryContext(ctx, `
SELECT datname
FROM pg_database
WHERE datallowconn AND NOT datistemplate AND has_database_privilege(datname, 'CONNECT')
ORDER BY datname`)
	if err != nil {
		return nil, fmt.Errorf("postgres: discover databases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result.Scopes = append(result.Scopes, metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name}))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	schemaRows, err := d.db.QueryContext(ctx, `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
  AND schema_name NOT LIKE 'pg_toast%'
  AND schema_name NOT LIKE 'pg_temp_%'
ORDER BY schema_name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: discover schemas: %w", err)
	}
	defer schemaRows.Close()
	for schemaRows.Next() {
		var name string
		if err := schemaRows.Scan(&name); err != nil {
			return nil, err
		}
		result.Scopes = append(result.Scopes, currentRoot.With("schema", name))
	}
	return result, schemaRows.Err()
}

// InspectObjects buckets refs by kind and composes RelationalObjects,
// MaterializedViewObjects, FunctionObjects, and SequenceObjects from
// catalog.go. A compatible engine overrides this method entirely to drop or
// add kinds.
func (d *Driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs, mvRefs, fnRefs, seqRefs []metadata.ObjectRef
	for _, r := range refs {
		switch r.Kind {
		case "table", "view":
			relRefs = append(relRefs, r)
		case "materialized_view":
			mvRefs = append(mvRefs, r)
		case "function":
			fnRefs = append(fnRefs, r)
		case "sequence":
			seqRefs = append(seqRefs, r)
		}
	}

	var out []metadata.Object
	if len(relRefs) > 0 {
		objs, err := RelationalObjects(ctx, d.db, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(mvRefs) > 0 {
		objs, err := MaterializedViewObjects(ctx, d.db, mvRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(fnRefs) > 0 {
		objs, err := FunctionObjects(ctx, d.db, fnRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(seqRefs) > 0 {
		objs, err := SequenceObjects(ctx, d.db, seqRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition serves one object's canonical text definition on demand so
// bulk InspectObjects (and every schema snapshot) skips the per-object cost:
// table DDL is reconstructed from the catalog, views come from pg_get_viewdef,
// and functions from pg_get_functiondef. Unsupported kinds, or an object that no
// longer exists, yield a nil descriptor with a nil error. A compatible engine
// overrides this method to special-case a kind and delegate everything else to
// this default via d.Driver.InspectDefinition(ctx, ref).
func (d *Driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	switch ref.Kind {
	case "table":
		ddl, err := TableDDL(ctx, d.db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", ddl), nil
	case "view":
		def, err := ViewDefinition(ctx, d.db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("Definition", "sql", def), nil
	case "function":
		language, def, err := FunctionDefinition(ctx, d.db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("Definition", language, def), nil
	default:
		return nil, nil
	}
}
