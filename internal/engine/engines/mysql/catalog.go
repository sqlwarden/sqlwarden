package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

// CatalogTables enumerates every table and view in database, invoking add
// once per object with its schema, name, and resolved kind ("table" or
// "view").
func CatalogTables(ctx context.Context, db *sql.DB, database string, add func(schema, name, kind string)) error {
	const q = `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema = ?
ORDER BY table_schema, table_name`
	rows, err := db.QueryContext(ctx, q, database)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ns, name, tableType string
		if err := rows.Scan(&ns, &name, &tableType); err != nil {
			return err
		}
		kind := "table"
		if tableType == "VIEW" {
			kind = "view"
		}
		add(ns, name, kind)
	}
	return rows.Err()
}

// CatalogRoutines enumerates every function and procedure in database,
// invoking add once per object with its resolved kind ("function" or
// "procedure").
func CatalogRoutines(ctx context.Context, db *sql.DB, database string, add func(schema, name, kind string)) error {
	const q = `
SELECT routine_schema, routine_name, routine_type
FROM information_schema.routines
WHERE routine_schema = ?
ORDER BY routine_type, routine_name`
	rows, err := db.QueryContext(ctx, q, database)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ns, name, routineType string
		if err := rows.Scan(&ns, &name, &routineType); err != nil {
			return err
		}
		kind := "procedure"
		if routineType == "FUNCTION" {
			kind = "function"
		}
		add(ns, name, kind)
	}
	return rows.Err()
}

// CatalogTriggers enumerates every trigger in database.
func CatalogTriggers(ctx context.Context, db *sql.DB, database string, add func(schema, name string)) error {
	const q = `
SELECT trigger_schema, trigger_name
FROM information_schema.triggers
WHERE trigger_schema = ?
ORDER BY trigger_name`
	rows, err := db.QueryContext(ctx, q, database)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ns, name string
		if err := rows.Scan(&ns, &name); err != nil {
			return err
		}
		add(ns, name)
	}
	return rows.Err()
}

// AttachRowCounts reports the approximate row count
// (information_schema.tables.table_rows) for every base table in database.
// It's an estimate refreshed by ANALYZE TABLE/InnoDB statistics, not a live
// COUNT(*), which is what keeps this query cheap regardless of table size.
func AttachRowCounts(ctx context.Context, db *sql.DB, database string, set func(name string, count int64)) error {
	const q = `
SELECT table_name, table_rows
FROM information_schema.tables
WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_rows IS NOT NULL`
	rows, err := db.QueryContext(ctx, q, database)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var rowCount int64
		if err := rows.Scan(&name, &rowCount); err != nil {
			return err
		}
		set(name, rowCount)
	}
	return rows.Err()
}

func mysqlPairFilter(refs []metadata.ObjectRef) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, len(refs)*2)
	for i, ref := range refs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?)")
		args = append(args, ref.Scope.Name("database"), ref.Name)
	}
	return sb.String(), args
}

// RelationalObjects fetches full column/PK/FK/index detail for tables and
// views named in refs.
func RelationalObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	refByName := make(map[string]metadata.ObjectRef, len(refs))
	for _, ref := range refs {
		refByName[ref.Scope.Name("database")+"\x00"+ref.Name] = ref
	}
	refFor := func(ns, name string) metadata.ObjectRef {
		return refByName[ns+"\x00"+name]
	}

	b := build.NewRelational()
	for _, ref := range refs {
		b.Ensure(ref)
	}

	pairs, args := mysqlPairFilter(refs)

	colQ := `
SELECT table_schema, table_name, column_name, column_type, is_nullable, column_default, ordinal_position,
       column_comment, extra
FROM information_schema.columns
WHERE (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, ordinal_position`
	crows, err := db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: object columns: %w", err)
	}
	for crows.Next() {
		var ns, tbl, col, dtype, nullable string
		var def sql.NullString
		var ord int
		var comment, extra string
		if err := crows.Scan(&ns, &tbl, &col, &dtype, &nullable, &def, &ord, &comment, &extra); err != nil {
			crows.Close()
			return nil, fmt.Errorf("mysql: object columns scan: %w", err)
		}
		c := metadata.Column{Name: col, DataType: dtype, Nullable: nullable == "YES", Ordinal: ord}
		if def.Valid {
			v := def.String
			c.Default = &v
		}
		setColumnAttr(&c, "comment", comment)
		setColumnAttr(&c, "extra", extra)
		b.AddColumn(refFor(ns, tbl), c)
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return nil, fmt.Errorf("mysql: object columns rows: %w", err)
	}
	crows.Close()

	pkQ := `
SELECT table_schema, table_name, column_name
FROM information_schema.key_column_usage
WHERE constraint_name = 'PRIMARY'
  AND (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, ordinal_position`
	prows, err := db.QueryContext(ctx, pkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: object pk: %w", err)
	}
	for prows.Next() {
		var ns, tbl, col string
		if err := prows.Scan(&ns, &tbl, &col); err != nil {
			prows.Close()
			return nil, fmt.Errorf("mysql: object pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(refFor(ns, tbl), col)
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, fmt.Errorf("mysql: object pk rows: %w", err)
	}
	prows.Close()

	fkQ := `
SELECT table_schema, table_name, constraint_name, column_name,
       referenced_table_schema, referenced_table_name, referenced_column_name
FROM information_schema.key_column_usage
WHERE referenced_table_schema IS NOT NULL
  AND referenced_table_name IS NOT NULL
  AND referenced_column_name IS NOT NULL
  AND (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, constraint_name, ordinal_position`
	frows, err := db.QueryContext(ctx, fkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: object fk: %w", err)
	}
	for frows.Next() {
		var ns, tbl, name, col, refNs, refTbl, refCol string
		if err := frows.Scan(&ns, &tbl, &name, &col, &refNs, &refTbl, &refCol); err != nil {
			frows.Close()
			return nil, fmt.Errorf("mysql: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(refFor(ns, tbl), name, col,
			metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: refNs}), Kind: "table", Name: refTbl}, refCol)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("mysql: object fk rows: %w", err)
	}
	frows.Close()

	idxQ := `
SELECT s.table_schema, s.table_name, s.index_name, s.non_unique, s.column_name, s.seq_in_index
FROM information_schema.statistics s
WHERE (s.table_schema, s.table_name) IN (` + pairs + `)
  AND s.index_name <> 'PRIMARY'
ORDER BY s.table_schema, s.table_name, s.index_name, s.seq_in_index`
	irows, err := db.QueryContext(ctx, idxQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: object indexes: %w", err)
	}
	type idxKey struct{ ns, tbl, name string }
	indexes := map[idxKey]*metadata.SecondaryIndex{}
	var indexOrder []idxKey
	for irows.Next() {
		var ns, tbl, name, col string
		var nonUnique int
		var seq int
		if err := irows.Scan(&ns, &tbl, &name, &nonUnique, &col, &seq); err != nil {
			irows.Close()
			return nil, fmt.Errorf("mysql: object index scan: %w", err)
		}
		key := idxKey{ns: ns, tbl: tbl, name: name}
		ix, ok := indexes[key]
		if !ok {
			ix = &metadata.SecondaryIndex{Name: name, Unique: nonUnique == 0}
			indexes[key] = ix
			indexOrder = append(indexOrder, key)
		}
		ix.Columns = append(ix.Columns, col)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return nil, fmt.Errorf("mysql: object index rows: %w", err)
	}
	irows.Close()
	for _, key := range indexOrder {
		b.AddIndex(refFor(key.ns, key.tbl), *indexes[key])
	}

	out := b.Build()
	if err := attachMySQLTableAttributes(ctx, db, out, pairs, args); err != nil {
		return nil, err
	}
	return out, nil
}

// attachMySQLTableAttributes populates each table object's engine, collation,
// and estimated row count from information_schema.tables.
func attachMySQLTableAttributes(ctx context.Context, db *sql.DB, objs []metadata.Object, pairs string, args []any) error {
	q := `
SELECT table_schema, table_name, engine, table_collation, table_rows
FROM information_schema.tables
WHERE (table_schema, table_name) IN (` + pairs + `)`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mysql: table attributes: %w", err)
	}
	defer rows.Close()

	type meta struct {
		engine    string
		collation string
		rows      string
	}
	byName := map[string]meta{}
	for rows.Next() {
		var ns, tbl string
		var engine, collation sql.NullString
		var tableRows sql.NullInt64
		if err := rows.Scan(&ns, &tbl, &engine, &collation, &tableRows); err != nil {
			return fmt.Errorf("mysql: table attributes scan: %w", err)
		}
		m := meta{engine: engine.String, collation: collation.String}
		if tableRows.Valid {
			m.rows = strconv.FormatInt(tableRows.Int64, 10)
		}
		byName[ns+"\x00"+tbl] = m
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mysql: table attributes rows: %w", err)
	}
	for i := range objs {
		m, ok := byName[objs[i].Ref.Scope.Name("database")+"\x00"+objs[i].Ref.Name]
		if !ok {
			continue
		}
		setObjectAttr(&objs[i], "engine", m.engine)
		setObjectAttr(&objs[i], "collation", m.collation)
		setObjectAttr(&objs[i], "row_estimate", m.rows)
	}
	return nil
}

func setColumnAttr(c *metadata.Column, key, value string) {
	if value == "" {
		return
	}
	if c.Attributes == nil {
		c.Attributes = map[string]any{}
	}
	c.Attributes[key] = value
}

func setObjectAttr(o *metadata.Object, key, value string) {
	if value == "" {
		return
	}
	if o.Attributes == nil {
		o.Attributes = map[string]any{}
	}
	o.Attributes[key] = value
}

// SourceDescriptor builds a "source" descriptor, or nil if body is empty (a
// dropped/inaccessible object rather than an error).
func SourceDescriptor(title, body string) *metadata.Descriptor {
	if body == "" {
		return nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: "sql", Body: body},
	}
}

// ShowCreateDefinition runs "<stmt><qualified ref>" and extracts the
// "Create ..." column as a source descriptor titled title. Returns (nil, nil)
// for a dropped object, an account without privileges, or a driver that lacks
// the requested SHOW CREATE variant — matching the other engines' lazy-
// definition contract. A compatible engine (e.g. MariaDB adding
// "SHOW CREATE SEQUENCE") calls this directly for its own kinds and delegates
// everything else to d.Driver.InspectDefinition.
func ShowCreateDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef, stmt, title string) (*metadata.Descriptor, error) {
	// MySQL cannot bind identifiers; mysqlQuoteQualified escapes both components.
	// codeql[go/sql-injection]
	rows, err := db.QueryContext(ctx, stmt+mysqlQuoteQualified(ref.Scope.Name("database"), ref.Name))
	if err != nil {
		return nil, nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("mysql: show create %s columns: %w", ref.Kind, err)
	}
	createIdx := -1
	for i, name := range cols {
		if strings.HasPrefix(name, "Create ") {
			createIdx = i
			break
		}
	}
	if createIdx < 0 {
		return nil, fmt.Errorf("mysql: show create %s: no Create column in %v", ref.Kind, cols)
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("mysql: show create %s rows: %w", ref.Kind, err)
		}
		return nil, nil
	}
	cells := make([]sql.NullString, len(cols))
	dest := make([]any, len(cols))
	for i := range cells {
		dest[i] = &cells[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, fmt.Errorf("mysql: show create %s scan: %w", ref.Kind, err)
	}
	return SourceDescriptor(title, cells[createIdx].String), nil
}

func mysqlQuoteQualified(namespace, name string) string {
	return mysqlQuoteIdent(namespace) + "." + mysqlQuoteIdent(name)
}

func mysqlQuoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
}

// RoutineObjects fetches signature detail for functions and procedures named
// in refs.
func RoutineObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	kindOf := make(map[string]string, len(refs))
	for _, ref := range refs {
		kindOf[ref.Scope.Name("database")+"\x00"+ref.Name] = ref.Kind
	}
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT routine_schema, routine_name, routine_type, data_type,
       external_language, sql_data_access, is_deterministic
FROM information_schema.routines
WHERE (routine_schema, routine_name) IN (` + pairs + `)
ORDER BY routine_schema, routine_name`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: routine detail: %w", err)
	}
	defer rows.Close()

	var out []metadata.Object
	for rows.Next() {
		var ns, name, routineType, sqlAccess, deterministic string
		var dataType, language sql.NullString
		if err := rows.Scan(&ns, &name, &routineType, &dataType, &language, &sqlAccess, &deterministic); err != nil {
			return nil, fmt.Errorf("mysql: routine detail scan: %w", err)
		}
		kind := kindOf[ns+"\x00"+name]
		if kind == "" {
			kind = strings.ToLower(routineType)
		}
		fields := []metadata.Field{
			{Name: "Type", Value: routineType},
			{Name: "SQL data access", Value: sqlAccess},
			{Name: "Deterministic", Value: deterministic},
		}
		if dataType.Valid && dataType.String != "" {
			fields = append(fields, metadata.Field{Name: "Returns", Value: dataType.String})
		}
		if language.Valid && language.String != "" {
			fields = append(fields, metadata.Field{Name: "Language", Value: language.String})
		}
		out = append(out, metadata.Object{
			Ref: mysqlRequestedRef(refs, ns, name, kind),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Routine", Fields: fields},
			},
		})
	}
	return out, rows.Err()
}

// TriggerObjects fetches detail for triggers named in refs.
func TriggerObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT trigger_schema, trigger_name, action_timing, event_manipulation,
       event_object_schema, event_object_table, action_statement
FROM information_schema.triggers
WHERE (trigger_schema, trigger_name) IN (` + pairs + `)
ORDER BY trigger_schema, trigger_name`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: trigger detail: %w", err)
	}
	defer rows.Close()

	var out []metadata.Object
	for rows.Next() {
		var ns, name, timing, event, tableNs, tableName, statement string
		if err := rows.Scan(&ns, &name, &timing, &event, &tableNs, &tableName, &statement); err != nil {
			return nil, fmt.Errorf("mysql: trigger detail scan: %w", err)
		}
		out = append(out, metadata.Object{
			Ref: mysqlRequestedRef(refs, ns, name, "trigger"),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Trigger", Fields: []metadata.Field{
					{Name: "Timing", Value: timing},
					{Name: "Event", Value: event},
					{Name: "Table", Value: tableNs + "." + tableName},
				}},
				{Kind: "source", Title: "Statement", Source: &metadata.Source{Language: "sql", Body: statement}},
			},
		})
	}
	return out, rows.Err()
}

func mysqlRequestedRef(refs []metadata.ObjectRef, database, name, kind string) metadata.ObjectRef {
	for _, ref := range refs {
		if ref.Scope.Name("database") == database && ref.Name == name {
			return ref
		}
	}
	return metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database}),
		Kind:  kind,
		Name:  name,
	}
}
