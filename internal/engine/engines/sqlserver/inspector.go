package sqlserver

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var (
	_ metadata.SchemaInspector     = (*Driver)(nil)
	_ metadata.ScopeDiscoverer     = (*Driver)(nil)
	_ metadata.DefinitionInspector = (*Driver)(nil)
)

func (d *Driver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "sqlserver",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated", HasDefinition: true},
			{Kind: "procedure", Label: "Procedure", PluralLabel: "Procedures", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
			{Kind: "trigger", Label: "Trigger", PluralLabel: "Triggers", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated", HasDefinition: true},
		},
	}
}

// InspectDirectory lists tables and views in the connection's current
// database, nested under a database root scope (SQL Server's directory
// hierarchy is database -> schema -> object). The database level is fixed
// to the connected database — SQL Server connections don't cross-query
// other databases the way Postgres can — so it is never enumerated as a
// browsable sibling list, only exposed as the root scope schemas nest
// under.
func (d *Driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	var database string
	var currentSchema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME(), SCHEMA_NAME()`).Scan(&database, &currentSchema); err != nil {
		return nil, fmt.Errorf("sqlserver: directory database context: %w", err)
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

	b := build.NewDirectory()
	b.AddScope(root)
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("procedure")
	b.DeclareKind("function")
	b.DeclareKind("trigger")

	addRef := func(schemaName, name, kind string) {
		scope := root.Child(metadata.ScopeSegment{Kind: "schema", Name: schemaName})
		b.AddRef(scope, kind, name)
	}
	if err := CatalogTables(ctx, d.db, addRef); err != nil {
		return nil, fmt.Errorf("sqlserver: catalog tables: %w", err)
	}
	if err := CatalogModules(ctx, d.db, addRef); err != nil {
		return nil, fmt.Errorf("sqlserver: catalog modules: %w", err)
	}

	return b.Build("", "sqlserver", defaultScope), nil
}

// DiscoverScopes lists schemas in the connection's current database,
// excluding SQL Server's built-in system schemas, nested under the
// connected database as the fixed root. The database level is never
// browsable to a sibling database (unlike Postgres): only requests for the
// top level or for this exact database's schema list are answered; anything
// past the schema level, or for a different database, returns no further
// scopes.
func (d *Driver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	var database string
	var currentSchema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME(), SCHEMA_NAME()`).Scan(&database, &currentSchema); err != nil {
		return nil, fmt.Errorf("sqlserver: discover current scope: %w", err)
	}
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	result := &metadata.ScopeDiscovery{Scopes: []metadata.ScopePath{}}
	if currentSchema.Valid {
		result.Current = root.Child(metadata.ScopeSegment{Kind: "schema", Name: currentSchema.String})
	}
	if request.Parent != "" {
		parentDatabase := request.Parent.Name("database")
		if parentDatabase != "" && parentDatabase != database {
			return result, nil
		}
		if request.Parent.Name("schema") != "" {
			return result, nil
		}
	}
	rows, err := d.db.QueryContext(ctx, `
SELECT name FROM sys.schemas
WHERE name NOT IN ('sys', 'INFORMATION_SCHEMA', 'guest', 'db_owner', 'db_accessadmin',
                    'db_securityadmin', 'db_ddladmin', 'db_backupoperator', 'db_datareader',
                    'db_datawriter', 'db_denydatareader', 'db_denydatawriter')
ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: discover schemas: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result.Scopes = append(result.Scopes, root.Child(metadata.ScopeSegment{Kind: "schema", Name: name}))
	}
	return result, rows.Err()
}

// InspectObjects buckets refs by kind: tables and views compose
// RelationalObjects, while procedures, functions, and triggers compose
// ModuleObjects (their T-SQL body is served on demand by InspectDefinition).
func (d *Driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs, moduleRefs []metadata.ObjectRef
	for _, ref := range refs {
		switch ref.Kind {
		case "table", "view":
			relRefs = append(relRefs, ref)
		case "procedure", "function", "trigger":
			moduleRefs = append(moduleRefs, ref)
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
	if len(moduleRefs) > 0 {
		objs, err := ModuleObjects(ctx, d.db, moduleRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition returns the object's canonical T-SQL text. Views,
// triggers, procedures, and functions are script-defined objects covered by
// sys.sql_modules.definition in one query — SQL Server has no per-object-type
// "SHOW CREATE" equivalent the way MySQL does. Tables have no module
// definition, so their DDL is reconstructed from the same column/PK/FK/index
// detail RelationalObjects already computes for the object viewer.
func (d *Driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	if ref.Kind == "table" {
		ddl, err := sqlServerTableDDL(ctx, d.db, ref)
		if err != nil {
			return nil, err
		}
		if ddl == "" {
			return nil, nil
		}
		return &metadata.Descriptor{
			Kind:   "source",
			Title:  "DDL",
			Source: &metadata.Source{Language: "sql", Body: ddl},
		}, nil
	}
	const q = `
SELECT m.definition
FROM sys.sql_modules m
JOIN sys.objects o ON o.object_id = m.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE s.name = @p1 AND o.name = @p2`
	var definition sql.NullString
	err := d.db.QueryRowContext(ctx, q, ref.Scope.Name("schema"), ref.Name).Scan(&definition)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlserver: inspect definition: %w", err)
	}
	if !definition.Valid || definition.String == "" {
		return nil, nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  "Definition",
		Source: &metadata.Source{Language: "sql", Body: definition.String},
	}, nil
}
