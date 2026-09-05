package mysql

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
		Dialect: "mysql",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "procedure", Label: "Procedure", PluralLabel: "Procedures", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "trigger", Label: "Trigger", PluralLabel: "Triggers", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

// InspectDirectory composes CatalogTables, AttachRowCounts, CatalogRoutines,
// and CatalogTriggers from catalog.go. A compatible engine that needs a
// different combination (e.g. no triggers) overrides this method entirely.
func (d *Driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	database := opts.Root.Name("database")
	if database == "" {
		database = d.defaultScope.Name("database")
	}
	if database == "" {
		var current sql.NullString
		if err := d.db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&current); err != nil {
			return nil, fmt.Errorf("mysql: directory database name: %w", err)
		}
		database = current.String
	}
	if database == "" {
		return &metadata.Directory{Engine: "mysql"}, nil
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	b := build.NewDirectory()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("function")
	b.DeclareKind("procedure")
	b.DeclareKind("trigger")

	if err := CatalogTables(ctx, d.db, database, func(ns, name, kind string) { b.AddRef(scope, kind, name) }); err != nil {
		return nil, fmt.Errorf("mysql: catalog tables: %w", err)
	}
	if err := AttachRowCounts(ctx, d.db, database, func(name string, count int64) { b.SetRowCount(scope, "table", name, count) }); err != nil {
		return nil, fmt.Errorf("mysql: catalog row counts: %w", err)
	}
	if err := CatalogRoutines(ctx, d.db, database, func(ns, name, kind string) { b.AddRef(scope, kind, name) }); err != nil {
		return nil, fmt.Errorf("mysql: catalog routines: %w", err)
	}
	if err := CatalogTriggers(ctx, d.db, database, func(ns, name string) { b.AddRef(scope, "trigger", name) }); err != nil {
		return nil, fmt.Errorf("mysql: catalog triggers: %w", err)
	}

	return b.Build("", "mysql", scope), nil
}

func (d *Driver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	var current sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&current); err != nil {
		return nil, fmt.Errorf("mysql: discover current database: %w", err)
	}
	result := &metadata.ScopeDiscovery{Scopes: []metadata.ScopePath{}}
	if current.Valid {
		result.Current = metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: current.String})
	}
	if request.Parent != "" {
		return result, nil
	}
	rows, err := d.db.QueryContext(ctx, `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('information_schema', 'mysql', 'performance_schema', 'sys')
ORDER BY schema_name`)
	if err != nil {
		return nil, fmt.Errorf("mysql: discover databases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result.Scopes = append(result.Scopes, metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name}))
	}
	return result, rows.Err()
}

// InspectObjects buckets refs by kind and composes RelationalObjects,
// RoutineObjects, and TriggerObjects from catalog.go. A compatible engine
// overrides this method entirely to drop or add kinds.
func (d *Driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs []metadata.ObjectRef
	var routineRefs []metadata.ObjectRef
	var triggerRefs []metadata.ObjectRef
	for _, ref := range refs {
		switch ref.Kind {
		case "table", "view":
			relRefs = append(relRefs, ref)
		case "function", "procedure":
			routineRefs = append(routineRefs, ref)
		case "trigger":
			triggerRefs = append(triggerRefs, ref)
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
	if len(routineRefs) > 0 {
		objs, err := RoutineObjects(ctx, d.db, routineRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(triggerRefs) > 0 {
		objs, err := TriggerObjects(ctx, d.db, triggerRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition serves one object's canonical text definition on demand via
// SHOW CREATE, so bulk InspectObjects (and every schema snapshot) skips the
// per-object SHOW CREATE TABLE round trip and the routine-body column it used to
// carry. Tables yield a "DDL" descriptor; views and routines yield "Definition".
// Unsupported kinds (e.g. triggers), or an object that no longer exists, yield a
// nil descriptor with a nil error. A compatible engine (e.g. MariaDB adding a
// "sequence" case) overrides this method, handles its own kinds, and delegates
// everything else to this default via d.Driver.InspectDefinition(ctx, ref).
func (d *Driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	var stmt, title string
	switch ref.Kind {
	case "table":
		stmt, title = "SHOW CREATE TABLE ", "DDL"
	case "view":
		stmt, title = "SHOW CREATE VIEW ", "Definition"
	case "function":
		stmt, title = "SHOW CREATE FUNCTION ", "Definition"
	case "procedure":
		stmt, title = "SHOW CREATE PROCEDURE ", "Definition"
	default:
		return nil, nil
	}
	return ShowCreateDefinition(ctx, d.db, ref, stmt, title)
}
