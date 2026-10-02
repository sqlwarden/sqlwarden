package sqlserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/engine/completioncore/mssql"
	"github.com/sqlwarden/internal/engine/metadata"
)

var (
	_ metadata.SchemaInspector = (*Driver)(nil)
	_ metadata.SessionScoper   = (*Driver)(nil)
)

func (d *Driver) Tree() metadata.Tree {
	return navigatorTree
}

// CurrentScope reports the session's database and the login's default schema.
func (d *Driver) CurrentScope(ctx context.Context) (metadata.ScopePath, error) {
	if d.db == nil {
		return "", errors.New("sqlserver: not connected")
	}
	var database, schema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME(), SCHEMA_NAME()`).Scan(&database, &schema); err != nil {
		return "", fmt.Errorf("sqlserver: read current scope: %w", err)
	}
	if !database.Valid || database.String == "" {
		return "", nil
	}
	path := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database.String})
	if schema.Valid && schema.String != "" {
		path = path.Child(metadata.ScopeSegment{Kind: "schema", Name: schema.String})
	}
	return path, nil
}

// Querier returns the session pool, switched to database per query. The sys
// catalog views only describe the current database, so loaders stay
// unqualified and run under USE instead.
func (d *Driver) Querier(_ context.Context, database string) (metadata.Querier, error) {
	if d.db == nil {
		return nil, errors.New("sqlserver: not connected")
	}
	if database == "" {
		return d.db, nil
	}
	return databaseQuerier{db: d.db, use: "USE " + sqlServerQuoteIdent(database) + ";\n"}, nil
}

// databaseQuerier prefixes every query with USE. go-mssqldb sends queries
// with arguments through sp_executesql, which scopes the USE to that call;
// argument-free queries go as a plain batch where the USE would hold until the
// pool resets the connection on its next checkout, so they get an unused
// argument to force sp_executesql.
type databaseQuerier struct {
	db  *sql.DB
	use string
}

func scopedArgs(args []any) []any {
	if len(args) == 0 {
		return []any{0}
	}
	return args
}

func (q databaseQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(ctx, q.use+query, scopedArgs(args)...)
}

func (q databaseQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return q.db.QueryRowContext(ctx, q.use+query, scopedArgs(args)...)
}

func folder(kind, label, child string, order int, list metadata.Loader, mixed ...string) metadata.Folder {
	return metadata.Folder{Kind: kind, Label: label, Child: child, MixedKinds: mixed, Order: order, List: list}
}

func leaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true}
}

func definedLeaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true, HasDefinition: true}
}

var (
	columnsFolder            = folder("columns", "Columns", "column", 10, ListColumns)
	uniqueKeysFolder         = folder("unique_keys", "Unique Keys", "unique_key", 20, ListUniqueKeys)
	checkConstraintsFolder   = folder("check_constraints", "Check Constraints", "check_constraint", 30, ListCheckConstraints)
	foreignKeysFolder        = folder("foreign_keys", "Foreign Keys", "foreign_key", 40, ListForeignKeys)
	indexesFolder            = folder("indexes", "Indexes", "index", 50, ListTableIndexes)
	referencesFolder         = folder("references", "References", "reference", 60, ListReferences)
	triggersFolder           = folder("triggers", "Triggers", "trigger", 70, ListTableTriggers)
	extendedPropertiesFolder = folder("extended_properties", "Extended Properties", "extended_property", 80, ListExtendedProperties)
)

var navigatorTree = metadata.Tree{
	SystemObjects:  true,
	FallbackScopes: []string{"dbo"},
	Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
		folder("databases", "Databases", "database", 10, ListDatabases),
		folder("logins", "Logins", "login", 20, ListLogins),
	}},
	Nodes: map[string]metadata.Node{
		"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, Folders: []metadata.Folder{
			folder("schemas", "Schemas", "schema", 10, ListSchemas),
			folder("database_triggers", "Database Triggers", "trigger", 20, ListDatabaseTriggers),
		}},
		"schema": {Label: "Schema", Icon: "schema", Scope: true, SupportsDiagram: true, Folders: []metadata.Folder{
			folder("tables", "Tables", "table", 10, ListTables),
			folder("external_tables", "External Tables", "external_table", 20, ListExternalTables),
			folder("views", "Views", "view", 30, ListViews),
			folder("indexes", "Indexes", "index", 40, ListSchemaIndexes),
			folder("procedures", "Procedures", "procedure", 50, ListProcedures, "function"),
			folder("sequences", "Sequences", "sequence", 60, ListSequences),
			folder("synonyms", "Synonyms", "synonym", 70, ListSynonyms),
			folder("triggers", "Triggers", "trigger", 80, ListSchemaTriggers),
			folder("data_types", "Data Types", "type", 90, ListDataTypes),
		}},
		"table": {Label: "Table", Icon: "table", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, uniqueKeysFolder, checkConstraintsFolder, foreignKeysFolder, indexesFolder, referencesFolder, triggersFolder, extendedPropertiesFolder,
		}},
		"external_table": {Label: "External Table", Icon: "foreign_table", Relational: true, Folders: []metadata.Folder{
			columnsFolder, extendedPropertiesFolder,
		}},
		"view": {Label: "View", Icon: "view", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, triggersFolder, extendedPropertiesFolder,
		}},
		"column":            {Label: "Column", Icon: "column", Leaf: true, Column: true},
		"unique_key":        leaf("Unique Key", "constraint"),
		"check_constraint":  leaf("Check Constraint", "constraint"),
		"foreign_key":       leaf("Foreign Key", "foreign_key"),
		"index":             leaf("Index", "index"),
		"reference":         leaf("Reference", "reference"),
		"trigger":           definedLeaf("Trigger", "trigger"),
		"extended_property": leaf("Extended Property", "extended_property"),
		"procedure":         definedLeaf("Procedure", "procedure"),
		"function":          definedLeaf("Function", "function"),
		"sequence":          leaf("Sequence", "sequence"),
		"synonym":           leaf("Synonym", "synonym"),
		"type":              leaf("Data Type", "type"),
		"login":             leaf("Login", "user"),
	},
}

func newListing(parents []metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
	out := make(map[metadata.ScopePath][]metadata.Child, len(parents))
	for _, p := range parents {
		out[p] = []metadata.Child{}
	}
	return out
}

// scanListing runs query and appends each scanned child under the parent the
// scan function resolves; rows for unknown parents are dropped.
func scanListing(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, args []any,
	scan func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: list %s: %w", label, err)
	}
	defer rows.Close()
	out := newListing(parents)
	for rows.Next() {
		parent, child, ok, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlserver: scan %s: %w", label, err)
		}
		if ok {
			out[parent] = append(out[parent], child)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlserver: list %s rows: %w", label, err)
	}
	return out, nil
}

// listShared runs one query and gives every parent the same children: root
// folders have the single parent "", and a database-level batch shares one
// database.
func listShared(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
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

// valuesList renders one parameter row per tuple, bound from @p1, for a
// derived VALUES table; T-SQL has no row-value IN.
func valuesList(tuples [][]string) (string, []any) {
	var sb strings.Builder
	var args []any
	for i, tuple := range tuples {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(")
		for j, v := range tuple {
			if j > 0 {
				sb.WriteString(",")
			}
			args = append(args, v)
			sb.WriteString("@p" + strconv.Itoa(len(args)))
		}
		sb.WriteString(")")
	}
	return sb.String(), args
}

// listInSchemas formats the requested schemas into query's `(VALUES %s) AS
// f(schema_name)` and runs it; the first column of every row is the schema
// name, which scan reads into schema along with the rest.
func listInSchemas(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows, schema *string) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	index := make(map[string]metadata.ScopePath, len(parents))
	tuples := make([][]string, 0, len(parents))
	for _, p := range parents {
		schema := p.Name("schema")
		if _, ok := index[schema]; !ok {
			tuples = append(tuples, []string{schema})
		}
		index[schema] = p
	}
	if len(tuples) == 0 {
		return newListing(parents), nil
	}
	values, args := valuesList(tuples)
	return scanListing(ctx, q, parents, label, fmt.Sprintf(query, values), args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var schema string
		child, err := scan(rows, &schema)
		if err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[schema]
		return parent, child, ok, nil
	})
}

type relationKey struct{ schema, name string }

// targetsCTE resolves the requested relations to object ids once so each
// relation-child query can join sys catalog views by object_id.
const targetsCTE = `
WITH targets AS (
  SELECT o.object_id, f.schema_name, f.object_name
  FROM (VALUES %s) AS f(schema_name, object_name)
  JOIN sys.schemas s ON s.name = f.schema_name
  JOIN sys.objects o ON o.schema_id = s.schema_id AND o.name = f.object_name
)`

// listRelationChildren runs targetsCTE + query for relation-level parents;
// the first two columns of every row are the parent's schema and relation
// name, scan reads the rest.
func listRelationChildren(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows, key *relationKey) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	index := make(map[relationKey]metadata.ScopePath, len(parents))
	tuples := make([][]string, 0, len(parents))
	for _, p := range parents {
		last, ok := p.Last()
		if !ok {
			continue
		}
		key := relationKey{schema: p.Name("schema"), name: last.Name}
		if _, dup := index[key]; !dup {
			tuples = append(tuples, []string{key.schema, key.name})
		}
		index[key] = p
	}
	if len(tuples) == 0 {
		return newListing(parents), nil
	}
	values, args := valuesList(tuples)
	return scanListing(ctx, q, parents, label, fmt.Sprintf(targetsCTE, values)+query, args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var key relationKey
		child, err := scan(rows, &key)
		if err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[key]
		return parent, child, ok, nil
	})
}

// qualifiedIn renders schema.name, or just name when it lives in home.
func qualifiedIn(home, schema, name string) string {
	if schema == home {
		return name
	}
	return schema + "." + name
}

const listDatabasesSQL = `
SELECT d.name, d.database_id, CASE WHEN d.name = DB_NAME() THEN 1 ELSE 0 END
FROM sys.databases d
WHERE d.state = 0 AND HAS_DBACCESS(d.name) = 1
ORDER BY d.name`

// ListDatabases lists the online databases the login can enter; master,
// tempdb, model, and msdb are system unless current.
func ListDatabases(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "databases", listDatabasesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var id, current int
		err := rows.Scan(&name, &id, &current)
		return metadata.Child{Kind: "database", Name: name, System: current != 1 && id <= 4, Current: current == 1}, err
	})
}

const listLoginsSQL = `
SELECT p.name, p.type_desc, p.is_disabled, CASE WHEN p.name = SUSER_SNAME() THEN 1 ELSE 0 END
FROM sys.server_principals p
WHERE p.type IN ('S', 'U', 'G', 'E', 'X', 'C', 'K')
ORDER BY p.name`

// ListLogins flags the certificate-mapped ##...## logins and the NT service
// accounts as system.
func ListLogins(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "logins", listLoginsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, loginType string
		var disabled bool
		var current int
		if err := rows.Scan(&name, &loginType, &disabled, &current); err != nil {
			return metadata.Child{}, err
		}
		system := strings.HasPrefix(name, "##") || strings.HasPrefix(name, `NT SERVICE\`) || strings.HasPrefix(name, `NT AUTHORITY\`)
		return metadata.Child{Kind: "login", Name: name, System: system, Current: current == 1, Attributes: map[string]any{
			"login_type": strings.ToLower(loginType), "disabled": disabled,
		}}, nil
	})
}

// listSchemasSQL marks the user's default schema current only in the
// database the session logged into: ORIGINAL_DB_NAME() is empty when the
// connection string names none, and the login's default database applies.
// Schema ids from 16384 are the fixed database role schemas (db_owner and
// friends).
const listSchemasSQL = `
SELECT s.name, CASE WHEN s.schema_id >= 16384 OR s.name IN ('sys', 'INFORMATION_SCHEMA', 'guest') THEN 1 ELSE 0 END,
       CASE WHEN s.name = SCHEMA_NAME() AND DB_NAME() = COALESCE(NULLIF(ORIGINAL_DB_NAME(), ''),
         (SELECT p.default_database_name FROM sys.server_principals p WHERE p.sid = SUSER_SID())) THEN 1 ELSE 0 END
FROM sys.schemas s
ORDER BY s.name`

func ListSchemas(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "schemas", listSchemasSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var system, current int
		err := rows.Scan(&name, &system, &current)
		return metadata.Child{Kind: "schema", Name: name, System: current != 1 && system == 1, Current: current == 1}, err
	})
}

const listDatabaseTriggersSQL = `
SELECT t.name, t.is_disabled, t.is_ms_shipped
FROM sys.triggers t
WHERE t.parent_class = 0
ORDER BY t.name`

func ListDatabaseTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "database triggers", listDatabaseTriggersSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var disabled, shipped bool
		err := rows.Scan(&name, &disabled, &shipped)
		return metadata.Child{Kind: "trigger", Name: name, System: shipped, Attributes: map[string]any{"disabled": disabled}}, err
	})
}

const listTablesSQL = `
SELECT f.schema_name, t.name, t.is_ms_shipped,
       (SELECT SUM(p.rows) FROM sys.partitions p WHERE p.object_id = t.object_id AND p.index_id IN (0, 1))
FROM sys.objects t
JOIN sys.schemas s ON s.schema_id = t.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
WHERE t.type = 'U'
ORDER BY s.name, t.name`

func ListTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "tables", listTablesSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name string
		var shipped bool
		var rowCount sql.NullInt64
		if err := rows.Scan(schema, &name, &shipped, &rowCount); err != nil {
			return metadata.Child{}, err
		}
		attrs := map[string]any{}
		if rowCount.Valid {
			attrs["row_count"] = rowCount.Int64
		}
		return metadata.Child{Kind: "table", Name: name, System: shipped, Attributes: attrs}, nil
	})
}

// listExternalTablesSQL reads sys.objects rather than sys.external_tables,
// which does not exist before SQL Server 2016.
const listExternalTablesSQL = `
SELECT f.schema_name, t.name
FROM sys.objects t
JOIN sys.schemas s ON s.schema_id = t.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
WHERE t.type = 'ET'
ORDER BY s.name, t.name`

func ListExternalTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "external tables", listExternalTablesSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		child := metadata.Child{Kind: "external_table"}
		err := rows.Scan(schema, &child.Name)
		return child, err
	})
}

const listViewsSQL = `
SELECT f.schema_name, v.name, v.is_ms_shipped
FROM sys.views v
JOIN sys.schemas s ON s.schema_id = v.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
ORDER BY s.name, v.name`

func ListViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "views", listViewsSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name string
		var shipped bool
		err := rows.Scan(schema, &name, &shipped)
		return metadata.Child{Kind: "view", Name: name, System: shipped}, err
	})
}

// listSchemaIndexesSQL skips heaps (index_id 0) and the hypothetical indexes
// the tuning advisor leaves behind.
const listSchemaIndexesSQL = `
SELECT f.schema_name, i.name, o.name, o.is_ms_shipped
FROM sys.indexes i
JOIN sys.objects o ON o.object_id = i.object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
WHERE i.index_id > 0 AND i.is_hypothetical = 0 AND o.type IN ('U', 'V')
ORDER BY s.name, i.name, o.name`

// ListSchemaIndexes lists each index name once: names are unique per table
// only, so the table attribute joins every table that has one by that name.
func ListSchemaIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out, err := listInSchemas(ctx, q, parents, "indexes", listSchemaIndexesSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name, table string
		var shipped bool
		err := rows.Scan(schema, &name, &table, &shipped)
		return metadata.Child{Kind: "index", Name: name, System: shipped, Attributes: map[string]any{"table": table}}, err
	})
	for parent, children := range out {
		out[parent] = mergeIndexes(children)
	}
	return out, err
}

// mergeIndexes collapses index rows that share a name, joining their tables;
// the merged index is system only when all its tables are. Rows arrive in
// collation order, so equal names need not be adjacent.
func mergeIndexes(children []metadata.Child) []metadata.Child {
	at := make(map[string]int, len(children))
	merged := children[:0]
	for _, c := range children {
		if i, ok := at[c.Name]; ok {
			merged[i].Attributes["table"] = merged[i].Attributes["table"].(string) + ", " + c.Attributes["table"].(string)
			merged[i].System = merged[i].System && c.System
			continue
		}
		at[c.Name] = len(merged)
		merged = append(merged, c)
	}
	return merged
}

// listRoutinesSQL covers T-SQL and CLR procedures (P, PC), extended
// procedures (X), and scalar, inline, table-valued, and CLR functions.
const listRoutinesSQL = `
SELECT f.schema_name, o.name, o.type, o.is_ms_shipped
FROM sys.objects o
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
WHERE o.type IN ('P', 'PC', 'X', 'FN', 'IF', 'TF', 'FS', 'FT')
ORDER BY s.name, o.name`

func ListProcedures(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "procedures", listRoutinesSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name, objectType string
		var shipped bool
		if err := rows.Scan(schema, &name, &objectType, &shipped); err != nil {
			return metadata.Child{}, err
		}
		child := metadata.Child{Kind: "function", Name: name, System: shipped}
		switch strings.TrimSpace(objectType) {
		case "P", "PC", "X":
			child.Kind = "procedure"
		case "IF", "TF", "FT":
			child.Attributes = map[string]any{mssql.ReturnsTableAttribute: true}
		}
		return child, nil
	})
}

const listSequencesSQL = `
SELECT f.schema_name, q.name, TYPE_NAME(q.user_type_id)
FROM sys.sequences q
JOIN sys.schemas s ON s.schema_id = q.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
ORDER BY s.name, q.name`

func ListSequences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "sequences", listSequencesSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name string
		var dataType sql.NullString
		err := rows.Scan(schema, &name, &dataType)
		return metadata.Child{Kind: "sequence", Name: name, Attributes: map[string]any{"data_type": dataType.String}}, err
	})
}

const listSynonymsSQL = `
SELECT f.schema_name, y.name, y.base_object_name
FROM sys.synonyms y
JOIN sys.schemas s ON s.schema_id = y.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
ORDER BY s.name, y.name`

func ListSynonyms(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "synonyms", listSynonymsSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name string
		var target sql.NullString
		err := rows.Scan(schema, &name, &target)
		return metadata.Child{Kind: "synonym", Name: name, Attributes: map[string]any{"target": target.String}}, err
	})
}

// listSchemaTriggersSQL lists DML triggers, which live in their parent
// object's schema.
const listSchemaTriggersSQL = `
SELECT f.schema_name, t.name, o.name, t.is_disabled, t.is_instead_of_trigger, t.is_ms_shipped
FROM sys.triggers t
JOIN sys.objects o ON o.object_id = t.parent_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
WHERE t.parent_class = 1
ORDER BY s.name, t.name`

func ListSchemaTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "triggers", listSchemaTriggersSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name, table string
		var disabled, insteadOf, shipped bool
		err := rows.Scan(schema, &name, &table, &disabled, &insteadOf, &shipped)
		return metadata.Child{Kind: "trigger", Name: name, System: shipped, Attributes: map[string]any{
			"table": table, "disabled": disabled, "instead_of": insteadOf,
		}}, err
	})
}

const listDataTypesSQL = `
SELECT f.schema_name, t.name, TYPE_NAME(t.system_type_id), t.is_table_type
FROM sys.types t
JOIN sys.schemas s ON s.schema_id = t.schema_id
JOIN (VALUES %s) AS f(schema_name) ON f.schema_name = s.name
WHERE t.is_user_defined = 1
ORDER BY s.name, t.name`

func ListDataTypes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchemas(ctx, q, parents, "data types", listDataTypesSQL, func(rows *sql.Rows, schema *string) (metadata.Child, error) {
		var name string
		var baseType sql.NullString
		var tableType bool
		err := rows.Scan(schema, &name, &baseType, &tableType)
		return metadata.Child{Kind: "type", Name: name, Attributes: map[string]any{"base_type": baseType.String, "table_type": tableType}}, err
	})
}

const listColumnsSQL = `
SELECT t.schema_name, t.object_name, c.name, ty.name, c.max_length, c.precision, c.scale, c.is_nullable, c.column_id,
       c.is_identity, c.is_computed,
       CASE WHEN EXISTS (
         SELECT 1 FROM sys.index_columns ic
         JOIN sys.indexes i ON i.object_id = ic.object_id AND i.index_id = ic.index_id
         WHERE i.is_primary_key = 1 AND ic.object_id = c.object_id AND ic.column_id = c.column_id
       ) THEN 1 ELSE 0 END,
       CASE WHEN EXISTS (
         SELECT 1 FROM sys.foreign_key_columns fkc
         WHERE fkc.parent_object_id = c.object_id AND fkc.parent_column_id = c.column_id
       ) THEN 1 ELSE 0 END
FROM targets t
JOIN sys.columns c ON c.object_id = t.object_id
JOIN sys.types ty ON ty.user_type_id = c.user_type_id
ORDER BY t.schema_name, t.object_name, c.column_id`

func ListColumns(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "columns", listColumnsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, typeName string
		var maxLength, precision, scale, ordinal, primaryKey, foreignKey int
		var nullable, identity, computed bool
		if err := rows.Scan(&key.schema, &key.name, &name, &typeName, &maxLength, &precision, &scale, &nullable, &ordinal,
			&identity, &computed, &primaryKey, &foreignKey); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "column", Name: name, Attributes: map[string]any{
			"data_type": sqlServerFormatType(typeName, maxLength, precision, scale), "nullable": nullable, "ordinal": ordinal,
			"primary_key": primaryKey == 1, "foreign_key": foreignKey == 1, "identity": identity, "computed": computed,
		}}, nil
	})
}

const listUniqueKeysSQL = `
SELECT t.schema_name, t.object_name, k.name, k.type
FROM targets t
JOIN sys.key_constraints k ON k.parent_object_id = t.object_id
ORDER BY t.schema_name, t.object_name, k.name`

func ListUniqueKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "unique keys", listUniqueKeysSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, keyType string
		if err := rows.Scan(&key.schema, &key.name, &name, &keyType); err != nil {
			return metadata.Child{}, err
		}
		constraintType := "unique"
		if strings.TrimSpace(keyType) == "PK" {
			constraintType = "primary_key"
		}
		return metadata.Child{Kind: "unique_key", Name: name, Attributes: map[string]any{"constraint_type": constraintType}}, nil
	})
}

const listCheckConstraintsSQL = `
SELECT t.schema_name, t.object_name, k.name, k.is_disabled
FROM targets t
JOIN sys.check_constraints k ON k.parent_object_id = t.object_id
ORDER BY t.schema_name, t.object_name, k.name`

func ListCheckConstraints(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "check constraints", listCheckConstraintsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var disabled bool
		err := rows.Scan(&key.schema, &key.name, &name, &disabled)
		return metadata.Child{Kind: "check_constraint", Name: name, Attributes: map[string]any{"disabled": disabled}}, err
	})
}

const listForeignKeysSQL = `
SELECT t.schema_name, t.object_name, k.name, OBJECT_SCHEMA_NAME(k.referenced_object_id) + '.' + OBJECT_NAME(k.referenced_object_id),
       k.delete_referential_action_desc
FROM targets t
JOIN sys.foreign_keys k ON k.parent_object_id = t.object_id
ORDER BY t.schema_name, t.object_name, k.name`

func ListForeignKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "foreign keys", listForeignKeysSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var target, deleteRule sql.NullString
		err := rows.Scan(&key.schema, &key.name, &name, &target, &deleteRule)
		return metadata.Child{Kind: "foreign_key", Name: name, Attributes: map[string]any{
			"referenced_table": target.String, "delete_rule": strings.ReplaceAll(strings.ToLower(deleteRule.String), "_", " "),
		}}, err
	})
}

const listTableIndexesSQL = `
SELECT t.schema_name, t.object_name, i.name, i.is_unique, i.is_primary_key, i.type_desc
FROM targets t
JOIN sys.indexes i ON i.object_id = t.object_id
WHERE i.index_id > 0 AND i.is_hypothetical = 0
ORDER BY t.schema_name, t.object_name, i.name`

func ListTableIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table indexes", listTableIndexesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, indexType string
		var unique, primary bool
		err := rows.Scan(&key.schema, &key.name, &name, &unique, &primary, &indexType)
		return metadata.Child{Kind: "index", Name: name, Attributes: map[string]any{
			"unique": unique, "primary": primary, "method": strings.ToLower(indexType),
		}}, err
	})
}

// listReferencesSQL names a referencing key by schema when it lives outside
// the referenced table's schema, since constraint names are unique per schema
// only.
const listReferencesSQL = `
SELECT t.schema_name, t.object_name, OBJECT_SCHEMA_NAME(k.object_id), k.name,
       OBJECT_SCHEMA_NAME(k.parent_object_id) + '.' + OBJECT_NAME(k.parent_object_id)
FROM targets t
JOIN sys.foreign_keys k ON k.referenced_object_id = t.object_id
ORDER BY t.schema_name, t.object_name, 3, k.name`

func ListReferences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "references", listReferencesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var schema, name, source string
		if err := rows.Scan(&key.schema, &key.name, &schema, &name, &source); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "reference", Name: qualifiedIn(key.schema, schema, name), Attributes: map[string]any{"source_table": source}}, nil
	})
}

const listTableTriggersSQL = `
SELECT t.schema_name, t.object_name, g.name, g.is_disabled, g.is_instead_of_trigger
FROM targets t
JOIN sys.triggers g ON g.parent_id = t.object_id
ORDER BY t.schema_name, t.object_name, g.name`

func ListTableTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table triggers", listTableTriggersSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var disabled, insteadOf bool
		err := rows.Scan(&key.schema, &key.name, &name, &disabled, &insteadOf)
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{"disabled": disabled, "instead_of": insteadOf}}, err
	})
}

// listExtendedPropertiesSQL lists the properties set on the object itself;
// class 1 with minor_id 0 excludes column-level properties.
const listExtendedPropertiesSQL = `
SELECT t.schema_name, t.object_name, p.name, CONVERT(nvarchar(4000), p.value)
FROM targets t
JOIN sys.extended_properties p ON p.class = 1 AND p.major_id = t.object_id AND p.minor_id = 0
ORDER BY t.schema_name, t.object_name, p.name`

func ListExtendedProperties(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "extended properties", listExtendedPropertiesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var value sql.NullString
		err := rows.Scan(&key.schema, &key.name, &name, &value)
		return metadata.Child{Kind: "extended_property", Name: name, Attributes: map[string]any{"value": value.String}}, err
	})
}
