package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*Driver)(nil)

func (d *Driver) Tree() metadata.Tree {
	return navigatorTree
}

func folder(kind, label, child string, order int, list metadata.Loader, mixed ...string) metadata.Folder {
	return metadata.Folder{Kind: kind, Label: label, Child: child, MixedKinds: mixed, Order: order, List: list}
}

func leaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true}
}

var (
	columnsFolder      = folder("columns", "Columns", "column", 10, ListColumns)
	constraintsFolder  = folder("constraints", "Constraints", "constraint", 20, ListConstraints)
	foreignKeysFolder  = folder("foreign_keys", "Foreign Keys", "foreign_key", 30, ListForeignKeys)
	indexesFolder      = folder("indexes", "Indexes", "index", 40, ListRelationIndexes)
	dependenciesFolder = folder("dependencies", "Dependencies", "dependency", 50, ListDependencies)
	referencesFolder   = folder("references", "References", "reference", 60, ListReferences)
	partitionsFolder   = folder("partitions", "Partitions", "partition", 70, ListPartitions)
	triggersFolder     = folder("triggers", "Triggers", "trigger", 80, ListTriggers)
	rulesFolder        = folder("rules", "Rules", "rule", 90, ListRules)
	policiesFolder     = folder("policies", "Policies", "policy", 100, ListPolicies)
)

var navigatorTree = metadata.Tree{
	SystemObjects: true,
	Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
		folder("databases", "Databases", "database", 10, ListDatabases),
	}},
	Nodes: map[string]metadata.Node{
		"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, Folders: []metadata.Folder{
			folder("schemas", "Schemas", "schema", 10, ListSchemas),
			folder("event_triggers", "Event Triggers", "event_trigger", 20, ListEventTriggers),
			folder("extensions", "Extensions", "extension", 30, ListExtensions),
		}},
		"schema": {Label: "Schema", Icon: "schema", Scope: true, SupportsDiagram: true, Folders: []metadata.Folder{
			folder("tables", "Tables", "table", 10, ListTables),
			folder("foreign_tables", "Foreign Tables", "foreign_table", 20, ListForeignTables),
			folder("views", "Views", "view", 30, ListViews),
			folder("materialized_views", "Materialized Views", "materialized_view", 40, ListMaterializedViews),
			folder("indexes", "Indexes", "index", 50, ListSchemaIndexes),
			folder("functions", "Functions", "function", 60, ListRoutines, "procedure"),
			folder("sequences", "Sequences", "sequence", 70, ListSequences),
			folder("data_types", "Data Types", "type", 80, ListDataTypes, "domain"),
			folder("aggregate_functions", "Aggregate Functions", "aggregate", 90, ListAggregates),
		}},
		"table": {Label: "Table", Icon: "table", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, constraintsFolder, foreignKeysFolder, indexesFolder, dependenciesFolder,
			referencesFolder, partitionsFolder, triggersFolder, rulesFolder, policiesFolder,
		}},
		"foreign_table": {Label: "Foreign Table", Icon: "foreign_table", Relational: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, constraintsFolder, foreignKeysFolder, indexesFolder, dependenciesFolder, referencesFolder, triggersFolder,
		}},
		"view": {Label: "View", Icon: "view", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, dependenciesFolder, triggersFolder, rulesFolder,
		}},
		"materialized_view": {Label: "Materialized View", Icon: "materialized_view", Relational: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, indexesFolder, dependenciesFolder,
		}},
		"column":        {Label: "Column", Icon: "column", Leaf: true, Column: true},
		"constraint":    leaf("Constraint", "constraint"),
		"foreign_key":   leaf("Foreign Key", "foreign_key"),
		"index":         leaf("Index", "index"),
		"dependency":    leaf("Dependency", "dependency"),
		"reference":     leaf("Reference", "reference"),
		"partition":     leaf("Partition", "partition"),
		"trigger":       leaf("Trigger", "trigger"),
		"rule":          leaf("Rule", "rule"),
		"policy":        leaf("Policy", "policy"),
		"function":      {Label: "Function", Icon: "function", Leaf: true, HasDefinition: true},
		"procedure":     {Label: "Procedure", Icon: "procedure", Leaf: true, HasDefinition: true},
		"sequence":      {Label: "Sequence", Icon: "sequence", Leaf: true, HasDefinition: true},
		"type":          {Label: "Data Type", Icon: "type", Leaf: true, HasDefinition: true},
		"domain":        {Label: "Domain", Icon: "domain", Leaf: true, HasDefinition: true},
		"aggregate":     leaf("Aggregate Function", "aggregate"),
		"event_trigger": leaf("Event Trigger", "event_trigger"),
		"extension":     leaf("Extension", "extension"),
	},
}

type relationKey struct{ schema, name string }

const relationParentsSQL = `
FROM unnest($1::text[], $2::text[]) AS p(nspname, relname)
JOIN pg_catalog.pg_namespace n ON n.nspname = p.nspname
JOIN pg_catalog.pg_class c ON c.relnamespace = n.oid AND c.relname = p.relname`

func newListing(parents []metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
	out := make(map[metadata.ScopePath][]metadata.Child, len(parents))
	for _, p := range parents {
		out[p] = []metadata.Child{}
	}
	return out
}

// schemaNames returns the distinct schema names of schema-level parents and a
// lookup from schema name back to the parent path.
func schemaNames(parents []metadata.ScopePath) ([]string, map[string]metadata.ScopePath) {
	names := make([]string, 0, len(parents))
	index := make(map[string]metadata.ScopePath, len(parents))
	for _, p := range parents {
		name := p.Name("schema")
		if _, seen := index[name]; !seen {
			names = append(names, name)
		}
		index[name] = p
	}
	return names, index
}

// relationPairs splits relation-level parents into parallel schema/name arrays
// for `unnest($1::text[], $2::text[])` and indexes them back to parent paths.
func relationPairs(parents []metadata.ScopePath) ([]string, []string, map[relationKey]metadata.ScopePath) {
	schemas := make([]string, 0, len(parents))
	names := make([]string, 0, len(parents))
	index := make(map[relationKey]metadata.ScopePath, len(parents))
	for _, p := range parents {
		last, ok := p.Last()
		if !ok {
			continue
		}
		key := relationKey{schema: p.Name("schema"), name: last.Name}
		schemas = append(schemas, key.schema)
		names = append(names, key.name)
		index[key] = p
	}
	return schemas, names, index
}

// scanListing runs query and appends each scanned child under the parent the
// scan function resolves; rows for unknown parents are dropped.
func scanListing(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, args []any,
	scan func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list %s: %w", label, err)
	}
	defer rows.Close()
	out := newListing(parents)
	for rows.Next() {
		parent, child, ok, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan %s: %w", label, err)
		}
		if ok {
			out[parent] = append(out[parent], child)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list %s rows: %w", label, err)
	}
	return out, nil
}

// listRelationChildren runs a relation-level query whose first two columns are
// the parent's schema and relation name; scan reads the remaining columns.
func listRelationChildren(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows, key *relationKey) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	schemas, names, index := relationPairs(parents)
	return scanListing(ctx, q, parents, label, query, []any{schemas, names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var key relationKey
		child, err := scan(rows, &key)
		if err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[key]
		return parent, child, ok, nil
	})
}

// listPerDatabase runs a database-wide query once and gives every parent the
// same children; parents of a loader batch always share one database.
func listPerDatabase(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	listed, err := scanListing(ctx, q, parents[:1], label, query, nil, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		child, err := scan(rows)
		return parents[0], child, true, err
	})
	if err != nil {
		return nil, err
	}
	out := newListing(parents)
	for _, p := range parents {
		out[p] = listed[parents[0]]
	}
	return out, nil
}

const listDatabasesSQL = `
SELECT d.datname, d.datname = current_database() AS is_current
FROM pg_catalog.pg_database d
WHERE d.datallowconn
  AND NOT d.datistemplate
  AND has_database_privilege(d.datname, 'CONNECT')
ORDER BY d.datname`

func ListDatabases(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listPerDatabase(ctx, q, parents, "databases", listDatabasesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "database"}
		err := rows.Scan(&child.Name, &child.Current)
		return child, err
	})
}

const listSchemasSQL = `
SELECT n.nspname,
       n.nspname IN ('pg_catalog', 'information_schema') OR n.nspname ~ '^pg_(toast|temp_)' AS is_system,
       COALESCE(n.nspname = current_schema(), false) AS is_current
FROM pg_catalog.pg_namespace n
WHERE has_schema_privilege(n.oid, 'USAGE')
ORDER BY n.nspname`

func ListSchemas(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listPerDatabase(ctx, q, parents, "schemas", listSchemasSQL, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "schema"}
		err := rows.Scan(&child.Name, &child.System, &child.Current)
		return child, err
	})
}

const listEventTriggersSQL = `
SELECT e.evtname, e.evtevent, e.evtenabled <> 'D' AS enabled
FROM pg_catalog.pg_event_trigger e
ORDER BY e.evtname`

func ListEventTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listPerDatabase(ctx, q, parents, "event triggers", listEventTriggersSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, event string
		var enabled bool
		err := rows.Scan(&name, &event, &enabled)
		return metadata.Child{Kind: "event_trigger", Name: name, Attributes: map[string]any{"event": event, "enabled": enabled}}, err
	})
}

const listExtensionsSQL = `
SELECT x.extname, x.extversion, n.nspname
FROM pg_catalog.pg_extension x
JOIN pg_catalog.pg_namespace n ON n.oid = x.extnamespace
ORDER BY x.extname`

func ListExtensions(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listPerDatabase(ctx, q, parents, "extensions", listExtensionsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, version, schema string
		err := rows.Scan(&name, &version, &schema)
		return metadata.Child{Kind: "extension", Name: name, Attributes: map[string]any{"version": version, "schema": schema}}, err
	})
}

const listTablesSQL = `
SELECT n.nspname, c.relname, c.relkind = 'p' AS partitioned, GREATEST(c.reltuples, 0)::bigint AS row_count
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1) AND c.relkind IN ('r', 'p') AND NOT c.relispartition
ORDER BY n.nspname, c.relname`

const listForeignTablesSQL = `
SELECT n.nspname, c.relname
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1) AND c.relkind = 'f'
ORDER BY n.nspname, c.relname`

const listViewsSQL = `
SELECT n.nspname, c.relname
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1) AND c.relkind = 'v'
ORDER BY n.nspname, c.relname`

const listMaterializedViewsSQL = `
SELECT n.nspname, c.relname, GREATEST(c.reltuples, 0)::bigint AS row_count
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1) AND c.relkind = 'm'
ORDER BY n.nspname, c.relname`

func ListTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, "tables", listTablesSQL, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name string
		var partitioned bool
		var rowCount int64
		if err := rows.Scan(&schema, &name, &partitioned, &rowCount); err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: "table", Name: name, Attributes: map[string]any{"row_count": rowCount, "partitioned": partitioned}}, ok, nil
	})
}

func ListForeignTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listSchemaRelations(ctx, q, parents, "foreign tables", listForeignTablesSQL, "foreign_table")
}

func ListViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listSchemaRelations(ctx, q, parents, "views", listViewsSQL, "view")
}

func ListMaterializedViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, "materialized views", listMaterializedViewsSQL, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name string
		var rowCount int64
		if err := rows.Scan(&schema, &name, &rowCount); err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: "materialized_view", Name: name, Attributes: map[string]any{"row_count": rowCount}}, ok, nil
	})
}

// listSchemaRelations serves schema-level folders whose query returns
// (schema, name) rows.
func listSchemaRelations(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query, kind string) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, label, query, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: kind, Name: name}, ok, nil
	})
}

const listSequencesSQL = `
SELECT n.nspname, c.relname
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = ANY($1) AND c.relkind = 'S'
ORDER BY n.nspname, c.relname`

func ListSequences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listSchemaRelations(ctx, q, parents, "sequences", listSequencesSQL, "sequence")
}

// listRoutinesSQL folds overloads into one child per name and kind; the
// object viewer resolves the individual signatures.
const listRoutinesSQL = `
SELECT n.nspname, p.proname, CASE p.prokind WHEN 'p' THEN 'procedure' ELSE 'function' END AS kind, count(*) AS overloads
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = ANY($1) AND p.prokind IN ('f', 'p')
GROUP BY n.nspname, p.proname, p.prokind
ORDER BY n.nspname, p.proname, 3`

const listAggregatesSQL = `
SELECT n.nspname, p.proname, count(*) AS overloads
FROM pg_catalog.pg_proc p
JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname = ANY($1) AND p.prokind = 'a'
GROUP BY n.nspname, p.proname
ORDER BY n.nspname, p.proname`

func ListRoutines(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, "routines", listRoutinesSQL, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name, kind string
		var overloads int64
		if err := rows.Scan(&schema, &name, &kind, &overloads); err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: kind, Name: name, Attributes: map[string]any{"overloads": overloads}}, ok, nil
	})
}

func ListAggregates(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, "aggregates", listAggregatesSQL, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name string
		var overloads int64
		if err := rows.Scan(&schema, &name, &overloads); err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: "aggregate", Name: name, Attributes: map[string]any{"overloads": overloads}}, ok, nil
	})
}

// listDataTypesSQL lists user-visible types: enums, ranges, domains, and
// standalone composites. Table row types and array types are excluded.
const listDataTypesSQL = `
SELECT n.nspname, t.typname, t.typtype::text
FROM pg_catalog.pg_type t
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
WHERE n.nspname = ANY($1)
  AND t.typtype IN ('c', 'd', 'e', 'r')
  AND (t.typtype <> 'c' OR EXISTS (
        SELECT 1 FROM pg_catalog.pg_class tc WHERE tc.oid = t.typrelid AND tc.relkind = 'c'))
ORDER BY n.nspname, t.typname`

var dataTypeKinds = map[string]string{"c": "composite", "d": "domain", "e": "enum", "r": "range"}

func ListDataTypes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, "data types", listDataTypesSQL, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name, typtype string
		if err := rows.Scan(&schema, &name, &typtype); err != nil {
			return "", metadata.Child{}, false, err
		}
		kind := "type"
		if typtype == "d" {
			kind = "domain"
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: kind, Name: name, Attributes: map[string]any{"type_kind": dataTypeKinds[typtype]}}, ok, nil
	})
}

const listRelationIndexesSQL = `
SELECT n.nspname, c.relname, i.relname, x.indisunique, x.indisprimary, am.amname` + relationParentsSQL + `
JOIN pg_catalog.pg_index x ON x.indrelid = c.oid
JOIN pg_catalog.pg_class i ON i.oid = x.indexrelid
JOIN pg_catalog.pg_am am ON am.oid = i.relam
ORDER BY n.nspname, c.relname, i.relname`

const listSchemaIndexesSQL = `
SELECT n.nspname, i.relname, t.relname, x.indisunique, x.indisprimary, am.amname
FROM pg_catalog.pg_namespace n
JOIN pg_catalog.pg_class i ON i.relnamespace = n.oid AND i.relkind IN ('i', 'I')
JOIN pg_catalog.pg_index x ON x.indexrelid = i.oid
JOIN pg_catalog.pg_class t ON t.oid = x.indrelid
JOIN pg_catalog.pg_am am ON am.oid = i.relam
WHERE n.nspname = ANY($1)
ORDER BY n.nspname, i.relname`

func ListRelationIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "indexes", listRelationIndexesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, method string
		var unique, primary bool
		if err := rows.Scan(&key.schema, &key.name, &name, &unique, &primary, &method); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "index", Name: name, Attributes: map[string]any{"unique": unique, "primary": primary, "method": method}}, nil
	})
}

func ListSchemaIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	names, index := schemaNames(parents)
	return scanListing(ctx, q, parents, "schema indexes", listSchemaIndexesSQL, []any{names}, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema, name, table, method string
		var unique, primary bool
		if err := rows.Scan(&schema, &name, &table, &unique, &primary, &method); err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, metadata.Child{Kind: "index", Name: name, Attributes: map[string]any{"table": table, "unique": unique, "primary": primary, "method": method}}, ok, nil
	})
}

const listColumnsSQL = `
SELECT n.nspname, c.relname, a.attname,
       pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
       NOT a.attnotnull AS nullable,
       a.attnum::bigint AS ordinal,
       EXISTS (SELECT 1 FROM pg_catalog.pg_constraint k
               WHERE k.conrelid = c.oid AND k.contype = 'p' AND a.attnum = ANY (k.conkey)) AS primary_key,
       EXISTS (SELECT 1 FROM pg_catalog.pg_constraint k
               WHERE k.conrelid = c.oid AND k.contype = 'f' AND a.attnum = ANY (k.conkey)) AS foreign_key` + relationParentsSQL + `
JOIN pg_catalog.pg_attribute a ON a.attrelid = c.oid
WHERE a.attnum > 0 AND NOT a.attisdropped
ORDER BY n.nspname, c.relname, a.attnum`

func ListColumns(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "columns", listColumnsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, dataType string
		var nullable, primaryKey, foreignKey bool
		var ordinal int64
		if err := rows.Scan(&key.schema, &key.name, &name, &dataType, &nullable, &ordinal, &primaryKey, &foreignKey); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "column", Name: name, Attributes: map[string]any{
			"data_type": dataType, "nullable": nullable, "ordinal": ordinal,
			"primary_key": primaryKey, "foreign_key": foreignKey,
		}}, nil
	})
}

const listConstraintsSQL = `
SELECT n.nspname, c.relname, k.conname, k.contype::text` + relationParentsSQL + `
JOIN pg_catalog.pg_constraint k ON k.conrelid = c.oid
WHERE k.contype IN ('p', 'u', 'c', 'x')
ORDER BY n.nspname, c.relname, k.conname`

const listForeignKeysSQL = `
SELECT n.nspname, c.relname, k.conname, k.confrelid::regclass::text` + relationParentsSQL + `
JOIN pg_catalog.pg_constraint k ON k.conrelid = c.oid AND k.contype = 'f'
ORDER BY n.nspname, c.relname, k.conname`

const listReferencesSQL = `
SELECT n.nspname, c.relname, k.conname, k.conrelid::regclass::text` + relationParentsSQL + `
JOIN pg_catalog.pg_constraint k ON k.confrelid = c.oid AND k.contype = 'f' AND k.conparentid = 0
ORDER BY n.nspname, c.relname, k.conname, 4`

var constraintTypes = map[string]string{"p": "primary_key", "u": "unique", "c": "check", "x": "exclusion"}

func ListConstraints(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "constraints", listConstraintsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, contype string
		if err := rows.Scan(&key.schema, &key.name, &name, &contype); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "constraint", Name: name, Attributes: map[string]any{"constraint_type": constraintTypes[contype]}}, nil
	})
}

func ListForeignKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "foreign keys", listForeignKeysSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, target string
		if err := rows.Scan(&key.schema, &key.name, &name, &target); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "foreign_key", Name: name, Attributes: map[string]any{"referenced_table": target}}, nil
	})
}

func ListReferences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "references", listReferencesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, source string
		if err := rows.Scan(&key.schema, &key.name, &name, &source); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "reference", Name: name, Attributes: map[string]any{"source_table": source}}, nil
	})
}

// listDependenciesSQL lists views and materialized views whose rewrite rule
// depends on the relation.
const listDependenciesSQL = `
SELECT DISTINCT n.nspname, c.relname, dn.nspname || '.' || dc.relname AS dependent,
       CASE dc.relkind WHEN 'm' THEN 'materialized_view' ELSE 'view' END AS object_kind` + relationParentsSQL + `
JOIN pg_catalog.pg_depend d ON d.refobjid = c.oid
  AND d.refclassid = 'pg_catalog.pg_class'::regclass
  AND d.classid = 'pg_catalog.pg_rewrite'::regclass
JOIN pg_catalog.pg_rewrite r ON r.oid = d.objid
JOIN pg_catalog.pg_class dc ON dc.oid = r.ev_class AND dc.oid <> c.oid AND dc.relkind IN ('v', 'm')
JOIN pg_catalog.pg_namespace dn ON dn.oid = dc.relnamespace
ORDER BY 1, 2, 3`

func ListDependencies(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "dependencies", listDependenciesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, kind string
		if err := rows.Scan(&key.schema, &key.name, &name, &kind); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "dependency", Name: name, Attributes: map[string]any{"object_kind": kind}}, nil
	})
}

const listPartitionsSQL = `
SELECT n.nspname, c.relname,
       CASE WHEN pn.oid = n.oid THEN pc.relname ELSE pn.nspname || '.' || pc.relname END AS partition_name,
       pg_catalog.pg_get_expr(pc.relpartbound, pc.oid) AS bound` + relationParentsSQL + `
JOIN pg_catalog.pg_inherits h ON h.inhparent = c.oid
JOIN pg_catalog.pg_class pc ON pc.oid = h.inhrelid AND pc.relispartition
JOIN pg_catalog.pg_namespace pn ON pn.oid = pc.relnamespace
ORDER BY 1, 2, 3`

func ListPartitions(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "partitions", listPartitionsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var bound sql.NullString
		if err := rows.Scan(&key.schema, &key.name, &name, &bound); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "partition", Name: name, Attributes: map[string]any{"bound": bound.String}}, nil
	})
}

const listTriggersSQL = `
SELECT n.nspname, c.relname, t.tgname, t.tgenabled <> 'D' AS enabled, t.tgfoid::regproc::text` + relationParentsSQL + `
JOIN pg_catalog.pg_trigger t ON t.tgrelid = c.oid AND NOT t.tgisinternal
ORDER BY n.nspname, c.relname, t.tgname`

func ListTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "triggers", listTriggersSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, function string
		var enabled bool
		if err := rows.Scan(&key.schema, &key.name, &name, &enabled, &function); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{"enabled": enabled, "function": function}}, nil
	})
}

const listRulesSQL = `
SELECT n.nspname, c.relname, r.rulename,
       CASE r.ev_type WHEN '1' THEN 'select' WHEN '2' THEN 'update' WHEN '3' THEN 'insert' WHEN '4' THEN 'delete' END AS event` + relationParentsSQL + `
JOIN pg_catalog.pg_rewrite r ON r.ev_class = c.oid AND r.rulename <> '_RETURN'
ORDER BY n.nspname, c.relname, r.rulename`

func ListRules(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "rules", listRulesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, event string
		if err := rows.Scan(&key.schema, &key.name, &name, &event); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "rule", Name: name, Attributes: map[string]any{"event": event}}, nil
	})
}

const listPoliciesSQL = `
SELECT n.nspname, c.relname, pol.polname,
       CASE pol.polcmd WHEN 'r' THEN 'select' WHEN 'a' THEN 'insert' WHEN 'w' THEN 'update' WHEN 'd' THEN 'delete' ELSE 'all' END AS command,
       pol.polpermissive` + relationParentsSQL + `
JOIN pg_catalog.pg_policy pol ON pol.polrelid = c.oid
ORDER BY n.nspname, c.relname, pol.polname`

func ListPolicies(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "policies", listPoliciesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, command string
		var permissive bool
		if err := rows.Scan(&key.schema, &key.name, &name, &command, &permissive); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "policy", Name: name, Attributes: map[string]any{"command": command, "permissive": permissive}}, nil
	})
}
