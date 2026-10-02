package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	moderncsqlite "modernc.org/sqlite"
)

var (
	_ metadata.SchemaInspector = (*sqliteDriver)(nil)
	_ metadata.SessionScoper   = (*sqliteDriver)(nil)
)

func (d *sqliteDriver) Tree() metadata.Tree {
	return navigatorTree
}

// CurrentScope is always main, the database the connection opened.
func (d *sqliteDriver) CurrentScope(context.Context) (metadata.ScopePath, error) {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}), nil
}

// Querier returns the connection pool for every database: catalog queries
// qualify by schema name, so one pool serves main, temp, and attached
// databases.
func (d *sqliteDriver) Querier(context.Context, string) (metadata.Querier, error) {
	if d.db == nil {
		return nil, errors.New("sqlite: not connected")
	}
	return d.db, nil
}

func folder(kind, label, child string, order int, list metadata.Loader) metadata.Folder {
	return metadata.Folder{Kind: kind, Label: label, Child: child, Order: order, List: list}
}

func leaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true}
}

var navigatorTree = metadata.Tree{
	SystemObjects: true,
	Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
		folder("databases", "Databases", "database", 10, ListDatabases),
	}},
	Nodes: map[string]metadata.Node{
		"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, SupportsDiagram: true, Folders: []metadata.Folder{
			folder("tables", "Tables", "table", 10, ListTables),
			folder("views", "Views", "view", 20, ListViews),
			folder("indexes", "Indexes", "index", 30, ListDatabaseIndexes),
			folder("sequences", "Sequences", "sequence", 40, ListSequences),
			folder("triggers", "Table Triggers", "trigger", 50, ListDatabaseTriggers),
			folder("data_types", "Data Types", "type", 60, ListDataTypes),
		}},
		"table": {Label: "Table", Icon: "table", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			folder("columns", "Columns", "column", 10, ListColumns),
			folder("keys", "Keys", "constraint", 20, ListKeys),
			folder("foreign_keys", "Foreign Keys", "foreign_key", 30, ListForeignKeys),
			folder("indexes", "Indexes", "index", 40, ListTableIndexes),
			folder("references", "References", "reference", 50, ListReferences),
			folder("triggers", "Triggers", "trigger", 60, ListTableTriggers),
		}},
		"view": {Label: "View", Icon: "view", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			folder("columns", "Columns", "column", 10, ListColumns),
		}},
		"column":      {Label: "Column", Icon: "column", Leaf: true, Column: true},
		"constraint":  leaf("Key", "constraint"),
		"foreign_key": leaf("Foreign Key", "foreign_key"),
		"reference":   leaf("Reference", "reference"),
		"index":       {Label: "Index", Icon: "index", Leaf: true, HasDefinition: true},
		"sequence":    leaf("Sequence", "sequence"),
		"trigger":     {Label: "Trigger", Icon: "trigger", Leaf: true, HasDefinition: true},
		"type":        leaf("Data Type", "type"),
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
// scan function resolves; rows for unknown parents are dropped. Schema
// qualifiers formatted into query are quoted by sqliteQuoteIdent because
// SQLite cannot bind them.
func scanListing(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, args []any,
	scan func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	// codeql[go/sql-injection]
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list %s: %w", label, err)
	}
	defer rows.Close()
	out := newListing(parents)
	for rows.Next() {
		parent, child, ok, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan %s: %w", label, err)
		}
		if ok {
			out[parent] = append(out[parent], child)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list %s rows: %w", label, err)
	}
	return out, nil
}

// listShared runs one query and gives every parent the same children. Root
// folders have the single parent ""; database folders' batches always share
// one database.
func listShared(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, args []any,
	scan func(rows *sql.Rows) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	listed, err := scanListing(ctx, q, parents[:1], label, query, args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
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

// listInDatabase binds parents[0]'s database as query's only argument: a
// batch's parents must share one database.
func listInDatabase(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	return listShared(ctx, q, parents, label, query, []any{parents[0].Name("database")}, scan)
}

// listInCatalog formats parents[0]'s quoted database into query's single %s,
// for catalog tables such as sqlite_master that have no pragma form.
func listInCatalog(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	return listShared(ctx, q, parents, label, fmt.Sprintf(query, sqliteQuoteIdent(parents[0].Name("database"))), nil, scan)
}

const listDatabasesSQL = `SELECT name FROM pragma_database_list ORDER BY seq`

// ListDatabases lists main, temp, and attached databases. temp is
// connection-private scratch space, so it is flagged system.
func ListDatabases(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "databases", listDatabasesSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "database"}
		err := rows.Scan(&child.Name)
		child.System = child.Name == "temp"
		child.Current = child.Name == "main"
		return child, err
	})
}

// listTablesSQL flags SQLite's own sqlite_* tables and virtual-table shadow
// tables as system.
const listTablesSQL = `
SELECT t.name, t.type, t.strict, t.name LIKE 'sqlite\_%' ESCAPE '\' OR t.type = 'shadow'
FROM pragma_table_list t
WHERE t.schema = ? AND t.type IN ('table', 'virtual', 'shadow')
ORDER BY t.name`

func ListTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "tables", listTablesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, tableType string
		var strict, system bool
		err := rows.Scan(&name, &tableType, &strict, &system)
		return metadata.Child{Kind: "table", Name: name, System: system, Attributes: map[string]any{"virtual": tableType == "virtual", "strict": strict}}, err
	})
}

const listViewsSQL = `
SELECT t.name
FROM pragma_table_list t
WHERE t.schema = ? AND t.type = 'view'
ORDER BY t.name`

func ListViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "views", listViewsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "view"}
		err := rows.Scan(&child.Name)
		return child, err
	})
}

// listDatabaseIndexesSQL flags the sqlite_autoindex_* indexes SQLite creates
// for primary key and unique constraints as system.
const listDatabaseIndexesSQL = `
SELECT i.name, t.name, i."unique", i.origin, i.origin <> 'c'
FROM pragma_table_list t
JOIN pragma_index_list(t.name, t.schema) i
WHERE t.schema = ? AND t.type = 'table'
ORDER BY i.name`

func ListDatabaseIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "indexes", listDatabaseIndexesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, table, origin string
		var unique, system bool
		err := rows.Scan(&name, &table, &unique, &origin, &system)
		return metadata.Child{Kind: "index", Name: name, System: system, Attributes: map[string]any{"table": table, "unique": unique, "origin": origin}}, err
	})
}

// sqlite_sequence exists only once a table declares AUTOINCREMENT.
const sequenceTableExistsSQL = `SELECT EXISTS (SELECT 1 FROM pragma_table_list WHERE schema = ? AND name = 'sqlite_sequence')`

// listSequencesSQL lists AUTOINCREMENT counters, one per table, named after
// the table they number. sqlite_sequence has no constraints and may be edited
// by hand, so NULL names are skipped and duplicate rows collapse.
const listSequencesSQL = `
SELECT s.name, max(s.seq)
FROM %s.sqlite_sequence s
WHERE s.name IS NOT NULL
GROUP BY s.name
ORDER BY s.name`

func ListSequences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	var exists bool
	if err := q.QueryRowContext(ctx, sequenceTableExistsSQL, parents[0].Name("database")).Scan(&exists); err != nil {
		return nil, fmt.Errorf("sqlite: list sequences: %w", err)
	}
	if !exists {
		return newListing(parents), nil
	}
	return listInCatalog(ctx, q, parents, "sequences", listSequencesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var value sql.NullInt64
		err := rows.Scan(&name, &value)
		return metadata.Child{Kind: "sequence", Name: name, Attributes: map[string]any{"value": value.Int64}}, err
	})
}

const listDatabaseTriggersSQL = `
SELECT m.name, m.tbl_name
FROM %s.sqlite_master m
WHERE m.type = 'trigger'
ORDER BY m.name`

func ListDatabaseTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInCatalog(ctx, q, parents, "triggers", listDatabaseTriggersSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, table string
		err := rows.Scan(&name, &table)
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{"table": table}}, err
	})
}

// dataTypes are SQLite's type affinities: every declared column type resolves
// to one of them, so the list is the same for every database.
var dataTypes = []string{"BLOB", "INTEGER", "NUMERIC", "REAL", "TEXT"}

func ListDataTypes(_ context.Context, _ metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out := newListing(parents)
	for _, p := range parents {
		for _, name := range dataTypes {
			out[p] = append(out[p], metadata.Child{Kind: "type", Name: name})
		}
	}
	return out, nil
}

// relationFilter renders relation-level parents as `(name, schema)` VALUES
// rows and indexes them back to parent paths by lowercased name, since SQLite
// matches table names case-insensitively.
func relationFilter(parents []metadata.ScopePath) (string, []any, map[string]metadata.ScopePath) {
	var sb strings.Builder
	args := make([]any, 0, len(parents)*2)
	index := make(map[string]metadata.ScopePath, len(parents))
	for _, p := range parents {
		last, ok := p.Last()
		if !ok {
			continue
		}
		if len(args) > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?)")
		args = append(args, last.Name, p.Name("database"))
		index[strings.ToLower(last.Name)] = p
	}
	return sb.String(), args, index
}

// listRelationChildren formats the parent VALUES rows into query as %[1]s and
// the batch's quoted database as %[2]s, then runs it. Every query binds the
// rows to a CTE `t(name, schema)`; the first column of every row is the
// parent relation name, scan reads the rest.
func listRelationChildren(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows, relation *string) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	filter, args, index := relationFilter(parents)
	if len(args) == 0 {
		return newListing(parents), nil
	}
	query = fmt.Sprintf(query, filter, sqliteQuoteIdent(parents[0].Name("database")))
	return scanListing(ctx, q, parents, label, query, args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var relation string
		child, err := scan(rows, &relation)
		if err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[strings.ToLower(relation)]
		return parent, child, ok, nil
	})
}

// listColumnsSQL skips hidden columns (virtual-table internals); hidden 2 and
// 3 are VIRTUAL and STORED generated columns.
const listColumnsSQL = `
WITH t(name, schema) AS (VALUES %[1]s)
SELECT t.name, p.name, p.type, p."notnull", p.pk, p.cid, p.hidden,
       EXISTS (SELECT 1 FROM pragma_foreign_key_list(t.name, t.schema) f WHERE f."from" = p.name)
FROM t
JOIN pragma_table_xinfo(t.name, t.schema) p
WHERE p.hidden <> 1
ORDER BY t.name, p.cid`

var generatedKinds = map[int]string{2: "virtual", 3: "stored"}

// sqliteErrorCode is SQLITE_ERROR, the primary result code for SQL errors such
// as a missing table.
const sqliteErrorCode = 1

func isSQLError(err error) bool {
	var sqliteErr *moderncsqlite.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqliteErrorCode
}

// ListColumns falls back to one query per parent when the batch fails with an
// SQL error: table_xinfo fails for a view over a dropped table, which must not
// hide the columns of the rest of the batch. Such a view lists no columns.
func ListColumns(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out, err := listColumns(ctx, q, parents)
	if !isSQLError(err) {
		return out, err
	}
	out = newListing(parents)
	for _, p := range parents {
		single, err := listColumns(ctx, q, []metadata.ScopePath{p})
		if isSQLError(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[p] = single[p]
	}
	return out, nil
}

func listColumns(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "columns", listColumnsSQL, func(rows *sql.Rows, relation *string) (metadata.Child, error) {
		var name, dataType string
		var notNull, foreignKey bool
		var pk, cid, hidden int
		if err := rows.Scan(relation, &name, &dataType, &notNull, &pk, &cid, &hidden, &foreignKey); err != nil {
			return metadata.Child{}, err
		}
		attrs := map[string]any{
			"data_type": dataType, "nullable": !notNull && pk == 0, "ordinal": cid + 1,
			"primary_key": pk > 0, "foreign_key": foreignKey,
		}
		if generated, ok := generatedKinds[hidden]; ok {
			attrs["generated"] = generated
		}
		return metadata.Child{Kind: "column", Name: name, Attributes: attrs}, nil
	})
}

// listKeysSQL reads the primary key from table_xinfo because a rowid alias
// (INTEGER PRIMARY KEY) has no backing index; unique constraints are the
// indexes SQLite created with origin 'u'. SQLite does not expose constraint
// names, so the primary key is named PRIMARY and unique keys by their index.
const listKeysSQL = `
WITH t(name, schema) AS (VALUES %[1]s)
SELECT t.name, 'PRIMARY', 'primary_key', group_concat(p.name, ', ' ORDER BY p.pk), 0
FROM t
JOIN pragma_table_xinfo(t.name, t.schema) p
WHERE p.pk > 0
GROUP BY t.name
UNION ALL
SELECT t.name, i.name, 'unique', group_concat(c.name, ', ' ORDER BY c.seqno), 1
FROM t
JOIN pragma_index_list(t.name, t.schema) i
JOIN pragma_index_info(i.name, t.schema) c
WHERE i.origin = 'u'
GROUP BY t.name, i.name
ORDER BY 1, 5, 2`

func ListKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "keys", listKeysSQL, func(rows *sql.Rows, relation *string) (metadata.Child, error) {
		var name, keyType string
		var columns sql.NullString
		var rank int
		if err := rows.Scan(relation, &name, &keyType, &columns, &rank); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "constraint", Name: name, Attributes: map[string]any{"constraint_type": keyType, "columns": columns.String}}, nil
	})
}

// listForeignKeysSQL groups multi-column keys by id. SQLite does not expose
// constraint names, so keys are named fk_<id> as in InspectObjects.
const listForeignKeysSQL = `
WITH t(name, schema) AS (VALUES %[1]s)
SELECT t.name, f.id, f."table", group_concat(f."from", ', ' ORDER BY f.seq), f.on_update, f.on_delete
FROM t
JOIN pragma_foreign_key_list(t.name, t.schema) f
GROUP BY t.name, f.id
ORDER BY t.name, f.id`

func ListForeignKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "foreign keys", listForeignKeysSQL, func(rows *sql.Rows, relation *string) (metadata.Child, error) {
		var id int
		var target, columns, onUpdate, onDelete string
		if err := rows.Scan(relation, &id, &target, &columns, &onUpdate, &onDelete); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "foreign_key", Name: fmt.Sprintf("fk_%d", id), Attributes: map[string]any{
			"referenced_table": target, "columns": columns, "on_update": onUpdate, "on_delete": onDelete,
		}}, nil
	})
}

// listReferencesSQL scans every table's foreign keys in the batch's database
// for ones targeting a parent. Foreign keys always target their own database.
const listReferencesSQL = `
WITH t(name, schema) AS (VALUES %[1]s)
SELECT f."table", s.name, f.id
FROM pragma_table_list s
JOIN pragma_foreign_key_list(s.name, s.schema) f
WHERE s.schema = (SELECT schema FROM t LIMIT 1) AND s.type = 'table' AND f.seq = 0
  AND f."table" COLLATE NOCASE IN (SELECT name FROM t)
ORDER BY 1, 2, 3`

func ListReferences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "references", listReferencesSQL, func(rows *sql.Rows, relation *string) (metadata.Child, error) {
		var source string
		var id int
		if err := rows.Scan(relation, &source, &id); err != nil {
			return metadata.Child{}, err
		}
		foreignKey := fmt.Sprintf("fk_%d", id)
		return metadata.Child{Kind: "reference", Name: source + "." + foreignKey, Attributes: map[string]any{"source_table": source, "foreign_key": foreignKey}}, nil
	})
}

const listTableTriggersSQL = `
WITH t(name, schema) AS (VALUES %[1]s)
SELECT m.tbl_name, m.name
FROM %[2]s.sqlite_master m
WHERE m.type = 'trigger' AND m.tbl_name COLLATE NOCASE IN (SELECT name FROM t)
ORDER BY m.tbl_name, m.name`

func ListTableTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table triggers", listTableTriggersSQL, func(rows *sql.Rows, relation *string) (metadata.Child, error) {
		child := metadata.Child{Kind: "trigger"}
		err := rows.Scan(relation, &child.Name)
		return child, err
	})
}

const listTableIndexesSQL = `
WITH t(name, schema) AS (VALUES %[1]s)
SELECT t.name, i.name, i."unique", i.origin, i.partial
FROM t
JOIN pragma_index_list(t.name, t.schema) i
ORDER BY t.name, i.name`

func ListTableIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table indexes", listTableIndexesSQL, func(rows *sql.Rows, relation *string) (metadata.Child, error) {
		var name, origin string
		var unique, partial bool
		if err := rows.Scan(relation, &name, &unique, &origin, &partial); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "index", Name: name, Attributes: map[string]any{"unique": unique, "primary": origin == "pk", "origin": origin, "partial": partial}}, nil
	})
}
