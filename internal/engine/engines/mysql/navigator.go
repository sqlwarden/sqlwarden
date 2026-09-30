package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*Driver)(nil)

func (d *Driver) Tree() metadata.Tree {
	return navigatorTree
}

// Querier returns the session pool for every database: MySQL catalog queries
// qualify by schema name, so one pool serves all databases.
func (d *Driver) Querier(context.Context, string) (metadata.Querier, error) {
	if d.db == nil {
		return nil, errors.New("mysql: not connected")
	}
	return d.db, nil
}

func folder(kind, label, child string, order int, list metadata.Loader, mixed ...string) metadata.Folder {
	return metadata.Folder{Kind: kind, Label: label, Child: child, MixedKinds: mixed, Order: order, List: list}
}

func leaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true}
}

var (
	columnsFolder     = folder("columns", "Columns", "column", 10, ListColumns)
	constraintsFolder = folder("constraints", "Constraints", "constraint", 20, ListConstraints)
	foreignKeysFolder = folder("foreign_keys", "Foreign Keys", "foreign_key", 30, ListForeignKeys)
	referencesFolder  = folder("references", "References", "reference", 40, ListReferences)
	triggersFolder    = folder("triggers", "Triggers", "trigger", 50, ListTableTriggers)
	indexesFolder     = folder("indexes", "Indexes", "index", 60, ListTableIndexes)
	partitionsFolder  = folder("partitions", "Partitions", "partition", 70, ListPartitions)
)

var navigatorTree = metadata.Tree{
	SystemObjects: true,
	Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
		folder("databases", "Databases", "database", 10, ListDatabases),
		folder("users", "Users", "user", 20, ListUsers),
	}},
	Nodes: map[string]metadata.Node{
		"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, SupportsDiagram: true, Folders: []metadata.Folder{
			folder("tables", "Tables", "table", 10, ListTables),
			folder("views", "Views", "view", 20, ListViews),
			folder("indexes", "Indexes", "index", 30, ListDatabaseIndexes),
			folder("procedures", "Procedures", "procedure", 40, ListRoutines, "function"),
			folder("triggers", "Triggers", "trigger", 50, ListDatabaseTriggers),
			folder("events", "Events", "event", 60, ListEvents),
		}},
		"table": {Label: "Table", Icon: "table", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, constraintsFolder, foreignKeysFolder, referencesFolder, triggersFolder, indexesFolder, partitionsFolder,
		}},
		"view": {Label: "View", Icon: "view", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder,
		}},
		"column":      {Label: "Column", Icon: "column", Leaf: true, Column: true},
		"constraint":  {Label: "Constraint", Icon: "constraint", Leaf: true, HasDefinition: true},
		"foreign_key": leaf("Foreign Key", "foreign_key"),
		"reference":   leaf("Reference", "reference"),
		"partition":   leaf("Partition", "partition"),
		"index":       {Label: "Index", Icon: "index", Leaf: true, HasDefinition: true},
		"procedure":   {Label: "Procedure", Icon: "procedure", Leaf: true, HasDefinition: true},
		"function":    {Label: "Function", Icon: "function", Leaf: true, HasDefinition: true},
		"trigger":     leaf("Trigger", "trigger"),
		"event":       {Label: "Event", Icon: "event", Leaf: true, HasDefinition: true},
		"user":        leaf("User", "user"),
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
		return nil, fmt.Errorf("mysql: list %s: %w", label, err)
	}
	defer rows.Close()
	out := newListing(parents)
	for rows.Next() {
		parent, child, ok, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("mysql: scan %s: %w", label, err)
		}
		if ok {
			out[parent] = append(out[parent], child)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: list %s rows: %w", label, err)
	}
	return out, nil
}

// listShared runs one query and gives every parent the same children. Root
// folders have the single parent ""; database folders' batches always share
// one database, which is bound as the query's only argument.
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

// listInDatabase binds only parents[0]'s database: a batch's parents must share one database.
func listInDatabase(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	return listShared(ctx, q, parents, label, query, []any{parents[0].Name("database")}, scan)
}

const listDatabasesSQL = `
SELECT s.schema_name,
       LOWER(s.schema_name) IN ('information_schema', 'mysql', 'performance_schema', 'sys') AS is_system,
       COALESCE(s.schema_name = DATABASE(), false) AS is_current
FROM information_schema.schemata s
ORDER BY s.schema_name`

func ListDatabases(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "databases", listDatabasesSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "database"}
		err := rows.Scan(&child.Name, &child.System, &child.Current)
		return child, err
	})
}

const listUsersSQL = `
SELECT u.user, u.host, u.user LIKE 'mysql.%' AS is_system
FROM mysql.user u
ORDER BY u.user, u.host`

// errTableAccessDenied is ER_TABLEACCESS_DENIED_ERROR, returned when the
// account lacks SELECT on mysql.user.
const errTableAccessDenied = 1142

// ListUsers lists accounts as user@host. Accounts without SELECT on
// mysql.user get an empty listing rather than an error.
func ListUsers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out, err := listShared(ctx, q, parents, "users", listUsersSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		var user, host string
		var system bool
		err := rows.Scan(&user, &host, &system)
		return metadata.Child{Kind: "user", Name: user + "@" + host, System: system, Attributes: map[string]any{"user": user, "host": host}}, err
	})
	var denied *mysqlconfig.MySQLError
	if errors.As(err, &denied) && denied.Number == errTableAccessDenied {
		return newListing(parents), nil
	}
	return out, err
}

const listTablesSQL = `
SELECT t.table_name, COALESCE(t.table_rows, 0), COALESCE(t.create_options LIKE '%partitioned%', false)
FROM information_schema.tables t
WHERE t.table_schema = ? AND t.table_type IN ('BASE TABLE', 'SYSTEM VERSIONED')
ORDER BY t.table_name`

func ListTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "tables", listTablesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var rowCount int64
		var partitioned bool
		err := rows.Scan(&name, &rowCount, &partitioned)
		return metadata.Child{Kind: "table", Name: name, Attributes: map[string]any{"row_count": rowCount, "partitioned": partitioned}}, err
	})
}

const listViewsSQL = `
SELECT t.table_name
FROM information_schema.tables t
WHERE t.table_schema = ? AND t.table_type IN ('VIEW', 'SYSTEM VIEW')
ORDER BY t.table_name`

func ListViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "views", listViewsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "view"}
		err := rows.Scan(&child.Name)
		return child, err
	})
}

// listDatabaseIndexesSQL lists each secondary index name once per database.
// MySQL index names are per table, so one name can span several tables; the
// table attribute joins them.
const listDatabaseIndexesSQL = `
SELECT s.index_name, GROUP_CONCAT(DISTINCT s.table_name ORDER BY s.table_name SEPARATOR ', ')
FROM information_schema.statistics s
WHERE s.table_schema = ? AND s.index_name <> 'PRIMARY'
GROUP BY s.index_name
ORDER BY s.index_name`

func ListDatabaseIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "indexes", listDatabaseIndexesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, table string
		err := rows.Scan(&name, &table)
		return metadata.Child{Kind: "index", Name: name, Attributes: map[string]any{"table": table}}, err
	})
}

const listRoutinesSQL = `
SELECT r.routine_name, r.routine_type
FROM information_schema.routines r
WHERE r.routine_schema = ? AND r.routine_type IN ('PROCEDURE', 'FUNCTION')
ORDER BY r.routine_name, r.routine_type`

func ListRoutines(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "routines", listRoutinesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, routineType string
		if err := rows.Scan(&name, &routineType); err != nil {
			return metadata.Child{}, err
		}
		kind := "procedure"
		if routineType == "FUNCTION" {
			kind = "function"
		}
		return metadata.Child{Kind: kind, Name: name}, nil
	})
}

const listDatabaseTriggersSQL = `
SELECT t.trigger_name, t.event_object_table, t.action_timing, t.event_manipulation
FROM information_schema.triggers t
WHERE t.trigger_schema = ?
ORDER BY t.trigger_name`

func ListDatabaseTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "triggers", listDatabaseTriggersSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, table, timing, event string
		err := rows.Scan(&name, &table, &timing, &event)
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{"table": table, "timing": timing, "event": event}}, err
	})
}

const listEventsSQL = `
SELECT e.event_name, e.status, e.event_type
FROM information_schema.events e
WHERE e.event_schema = ?
ORDER BY e.event_name`

func ListEvents(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "events", listEventsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, status, eventType string
		err := rows.Scan(&name, &status, &eventType)
		return metadata.Child{Kind: "event", Name: name, Attributes: map[string]any{"status": status, "event_type": eventType}}, err
	})
}

const listSequencesSQL = `
SELECT t.table_name
FROM information_schema.tables t
WHERE t.table_schema = ? AND t.table_type = 'SEQUENCE'
ORDER BY t.table_name`

// ListSequences lists native sequences for MySQL-family engines that have
// them (MariaDB, TiDB). MySQL itself declares no sequences folder.
func ListSequences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInDatabase(ctx, q, parents, "sequences", listSequencesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "sequence"}
		err := rows.Scan(&child.Name)
		return child, err
	})
}

type relationKey struct{ database, name string }

// relationFilter renders relation-level parents as a row-constructor list for
// `(schema, table) IN (%s)` and indexes them back to parent paths.
func relationFilter(parents []metadata.ScopePath) (string, []any, map[relationKey]metadata.ScopePath) {
	var sb strings.Builder
	args := make([]any, 0, len(parents)*2)
	index := make(map[relationKey]metadata.ScopePath, len(parents))
	for _, p := range parents {
		last, ok := p.Last()
		if !ok {
			continue
		}
		key := relationKey{database: p.Name("database"), name: last.Name}
		if len(args) > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?)")
		args = append(args, key.database, key.name)
		index[key] = p
	}
	return sb.String(), args, index
}

// listRelationChildren formats the parent filter into query's single %s and
// runs it; the first two columns of every row are the parent's database and
// relation name, scan reads the rest.
func listRelationChildren(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows, key *relationKey) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	filter, args, index := relationFilter(parents)
	if len(args) == 0 {
		return newListing(parents), nil
	}
	return scanListing(ctx, q, parents, label, fmt.Sprintf(query, filter), args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var key relationKey
		child, err := scan(rows, &key)
		if err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[key]
		return parent, child, ok, nil
	})
}

const listColumnsSQL = `
SELECT c.table_schema, c.table_name, c.column_name, c.column_type,
       c.is_nullable = 'YES', c.ordinal_position, c.column_key = 'PRI',
       EXISTS (SELECT 1 FROM information_schema.key_column_usage k
               WHERE k.table_schema = c.table_schema AND k.table_name = c.table_name
                 AND k.column_name = c.column_name AND k.referenced_table_name IS NOT NULL)
FROM information_schema.columns c
WHERE (c.table_schema, c.table_name) IN (%s)
ORDER BY c.table_schema, c.table_name, c.ordinal_position`

func ListColumns(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "columns", listColumnsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, dataType string
		var nullable, primaryKey, foreignKey bool
		var ordinal int64
		if err := rows.Scan(&key.database, &key.name, &name, &dataType, &nullable, &ordinal, &primaryKey, &foreignKey); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "column", Name: name, Attributes: map[string]any{
			"data_type": dataType, "nullable": nullable, "ordinal": ordinal,
			"primary_key": primaryKey, "foreign_key": foreignKey,
		}}, nil
	})
}

var constraintTypes = map[string]string{"PRIMARY KEY": "primary_key", "UNIQUE": "unique", "CHECK": "check"}

const listConstraintsSQL = `
SELECT tc.table_schema, tc.table_name, tc.constraint_name, tc.constraint_type
FROM information_schema.table_constraints tc
WHERE (tc.table_schema, tc.table_name) IN (%s) AND tc.constraint_type IN ('PRIMARY KEY', 'UNIQUE', 'CHECK')
ORDER BY tc.table_schema, tc.table_name, tc.constraint_name`

func ListConstraints(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "constraints", listConstraintsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, constraintType string
		if err := rows.Scan(&key.database, &key.name, &name, &constraintType); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "constraint", Name: name, Attributes: map[string]any{"constraint_type": constraintTypes[constraintType]}}, nil
	})
}

const listForeignKeysSQL = `
SELECT rc.constraint_schema, rc.table_name, rc.constraint_name,
       CONCAT(rc.unique_constraint_schema, '.', rc.referenced_table_name)
FROM information_schema.referential_constraints rc
WHERE (rc.constraint_schema, rc.table_name) IN (%s)
ORDER BY rc.constraint_schema, rc.table_name, rc.constraint_name`

func ListForeignKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "foreign keys", listForeignKeysSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, target string
		if err := rows.Scan(&key.database, &key.name, &name, &target); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "foreign_key", Name: name, Attributes: map[string]any{"referenced_table": target}}, nil
	})
}

const listReferencesSQL = `
SELECT rc.unique_constraint_schema, rc.referenced_table_name, rc.constraint_name,
       CONCAT(rc.constraint_schema, '.', rc.table_name)
FROM information_schema.referential_constraints rc
WHERE (rc.unique_constraint_schema, rc.referenced_table_name) IN (%s)
ORDER BY rc.unique_constraint_schema, rc.referenced_table_name, rc.constraint_name, 4`

func ListReferences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "references", listReferencesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, source string
		if err := rows.Scan(&key.database, &key.name, &name, &source); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "reference", Name: name, Attributes: map[string]any{"source_table": source}}, nil
	})
}

const listTableTriggersSQL = `
SELECT t.event_object_schema, t.event_object_table, t.trigger_name, t.action_timing, t.event_manipulation
FROM information_schema.triggers t
WHERE (t.event_object_schema, t.event_object_table) IN (%s)
ORDER BY t.event_object_schema, t.event_object_table, t.trigger_name`

func ListTableTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table triggers", listTableTriggersSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, timing, event string
		if err := rows.Scan(&key.database, &key.name, &name, &timing, &event); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{"timing": timing, "event": event}}, nil
	})
}

const listTableIndexesSQL = `
SELECT s.table_schema, s.table_name, s.index_name, MIN(s.non_unique) = 0, LOWER(MIN(s.index_type))
FROM information_schema.statistics s
WHERE (s.table_schema, s.table_name) IN (%s)
GROUP BY s.table_schema, s.table_name, s.index_name
ORDER BY s.table_schema, s.table_name, s.index_name`

func ListTableIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table indexes", listTableIndexesSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, method string
		var unique bool
		if err := rows.Scan(&key.database, &key.name, &name, &unique, &method); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "index", Name: name, Attributes: map[string]any{"unique": unique, "primary": name == "PRIMARY", "method": method}}, nil
	})
}

// listPartitionsSQL collapses subpartition rows to one row per partition.
const listPartitionsSQL = `
SELECT p.table_schema, p.table_name, p.partition_name,
       MIN(p.partition_method), MIN(p.partition_description), MIN(p.partition_ordinal_position)
FROM information_schema.partitions p
WHERE (p.table_schema, p.table_name) IN (%s) AND p.partition_name IS NOT NULL
GROUP BY p.table_schema, p.table_name, p.partition_name
ORDER BY p.table_schema, p.table_name, 6`

func ListPartitions(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "partitions", listPartitionsSQL, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var method, bound sql.NullString
		var ordinal int64
		if err := rows.Scan(&key.database, &key.name, &name, &method, &bound, &ordinal); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "partition", Name: name, Attributes: map[string]any{"method": method.String, "bound": bound.String}}, nil
	})
}
