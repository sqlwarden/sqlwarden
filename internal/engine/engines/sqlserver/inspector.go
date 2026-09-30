package sqlserver

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

var (
	_ metadata.DefinitionInspector = (*Driver)(nil)
)

// InspectObjects buckets refs by database, then by kind: tables, views, and
// external tables compose RelationalObjects, while procedures, functions, and
// triggers compose ModuleObjects (their T-SQL body is served on demand by
// InspectDefinition).
func (d *Driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var databases []string
	byDatabase := map[string][]metadata.ObjectRef{}
	for _, ref := range refs {
		database := ref.Scope.Name("database")
		if _, ok := byDatabase[database]; !ok {
			databases = append(databases, database)
		}
		byDatabase[database] = append(byDatabase[database], ref)
	}
	var out []metadata.Object
	for _, database := range databases {
		q, err := d.Querier(ctx, database)
		if err != nil {
			return nil, err
		}
		objs, err := inspectObjectsIn(ctx, q, byDatabase[database])
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

func inspectObjectsIn(ctx context.Context, q metadata.Querier, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs, moduleRefs []metadata.ObjectRef
	for _, ref := range refs {
		switch ref.Kind {
		case "table", "view", "external_table":
			relRefs = append(relRefs, ref)
		case "procedure", "function", "trigger":
			moduleRefs = append(moduleRefs, ref)
		}
	}

	var out []metadata.Object
	if len(relRefs) > 0 {
		objs, err := RelationalObjects(ctx, q, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(moduleRefs) > 0 {
		objs, err := ModuleObjects(ctx, q, moduleRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

const moduleDefinitionSQL = `
SELECT m.definition
FROM sys.sql_modules m
JOIN sys.objects o ON o.object_id = m.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
WHERE s.name = @p1 AND o.name = @p2`

// databaseTriggerDefinitionSQL reads DDL triggers, which are database-scoped
// and absent from sys.objects.
const databaseTriggerDefinitionSQL = `
SELECT m.definition
FROM sys.sql_modules m
JOIN sys.triggers t ON t.object_id = m.object_id
WHERE t.parent_class = 0 AND t.name = @p1`

// InspectDefinition returns the object's canonical T-SQL text from the
// database named in ref's scope. Views, triggers, procedures, and functions
// are script-defined objects covered by sys.sql_modules.definition in one
// query; SQL Server has no per-object-type "SHOW CREATE" equivalent the way
// MySQL does. Tables have no module definition, so their DDL is
// reconstructed from the same column/PK/FK/index detail RelationalObjects
// already computes for the object viewer.
func (d *Driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	q, err := d.Querier(ctx, ref.Scope.Name("database"))
	if err != nil {
		return nil, err
	}
	if ref.Kind == "table" {
		ddl, err := sqlServerTableDDL(ctx, q, ref)
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
	var definition sql.NullString
	schema := ref.Scope.Name("schema")
	if schema == "" && ref.Kind == "trigger" {
		err = q.QueryRowContext(ctx, databaseTriggerDefinitionSQL, ref.Name).Scan(&definition)
	} else {
		err = q.QueryRowContext(ctx, moduleDefinitionSQL, schema, ref.Name).Scan(&definition)
	}
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
