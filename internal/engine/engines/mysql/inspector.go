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

var _ metadata.SchemaInspector = (*mysqlDriver)(nil)
var _ metadata.ScopeDiscoverer = (*mysqlDriver)(nil)
var _ metadata.DefinitionInspector = (*mysqlDriver)(nil)

func (d *mysqlDriver) SchemaSpec() metadata.SchemaSpec {
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

func (d *mysqlDriver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
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

	q := `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema = ?
ORDER BY table_schema, table_name`
	rows, err := d.db.QueryContext(ctx, q, database)
	if err != nil {
		return nil, fmt.Errorf("mysql: catalog tables: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ns, name, tableType string
		if err := rows.Scan(&ns, &name, &tableType); err != nil {
			return nil, fmt.Errorf("mysql: catalog tables scan: %w", err)
		}
		kind := "table"
		if tableType == "VIEW" {
			kind = "view"
		}
		b.AddRef(scope, kind, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: catalog tables rows: %w", err)
	}
	rows.Close()

	if err := d.attachRowCounts(ctx, b, scope, database); err != nil {
		return nil, fmt.Errorf("mysql: catalog row counts: %w", err)
	}

	const routineQ = `
SELECT routine_schema, routine_name, routine_type
FROM information_schema.routines
WHERE routine_schema = ?
ORDER BY routine_type, routine_name`
	routineRows, err := d.db.QueryContext(ctx, routineQ, database)
	if err != nil {
		return nil, fmt.Errorf("mysql: catalog routines: %w", err)
	}
	for routineRows.Next() {
		var ns, name, routineType string
		if err := routineRows.Scan(&ns, &name, &routineType); err != nil {
			routineRows.Close()
			return nil, fmt.Errorf("mysql: catalog routines scan: %w", err)
		}
		kind := "procedure"
		if routineType == "FUNCTION" {
			kind = "function"
		}
		b.AddRef(scope, kind, name)
	}
	if err := routineRows.Err(); err != nil {
		routineRows.Close()
		return nil, fmt.Errorf("mysql: catalog routines rows: %w", err)
	}
	routineRows.Close()

	const triggerQ = `
SELECT trigger_schema, trigger_name
FROM information_schema.triggers
WHERE trigger_schema = ?
ORDER BY trigger_name`
	triggerRows, err := d.db.QueryContext(ctx, triggerQ, database)
	if err != nil {
		return nil, fmt.Errorf("mysql: catalog triggers: %w", err)
	}
	for triggerRows.Next() {
		var ns, name string
		if err := triggerRows.Scan(&ns, &name); err != nil {
			triggerRows.Close()
			return nil, fmt.Errorf("mysql: catalog triggers scan: %w", err)
		}
		b.AddRef(scope, "trigger", name)
	}
	if err := triggerRows.Err(); err != nil {
		triggerRows.Close()
		return nil, fmt.Errorf("mysql: catalog triggers rows: %w", err)
	}
	triggerRows.Close()

	return b.Build("", "mysql", scope), nil
}

// attachRowCounts sets the approximate row count (information_schema.tables.
// table_rows) for every base table. It's an estimate refreshed by ANALYZE
// TABLE/InnoDB statistics, not a live COUNT(*), which is what keeps this
// query cheap regardless of table size.
func (d *mysqlDriver) attachRowCounts(ctx context.Context, b *build.DirectoryBuilder, scope metadata.ScopePath, database string) error {
	const q = `
SELECT table_name, table_rows
FROM information_schema.tables
WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_rows IS NOT NULL`
	rows, err := d.db.QueryContext(ctx, q, database)
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
		b.SetRowCount(scope, "table", name, rowCount)
	}
	return rows.Err()
}

func (d *mysqlDriver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
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

func (d *mysqlDriver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
		objs, err := d.inspectRelational(ctx, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(routineRefs) > 0 {
		objs, err := d.inspectRoutines(ctx, routineRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(triggerRefs) > 0 {
		objs, err := d.inspectTriggers(ctx, triggerRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
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

func (d *mysqlDriver) inspectRelational(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
	crows, err := d.db.QueryContext(ctx, colQ, args...)
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
	prows, err := d.db.QueryContext(ctx, pkQ, args...)
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
	frows, err := d.db.QueryContext(ctx, fkQ, args...)
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
	irows, err := d.db.QueryContext(ctx, idxQ, args...)
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
	if err := d.attachMySQLTableAttributes(ctx, out, pairs, args); err != nil {
		return nil, err
	}
	return out, nil
}

// attachMySQLTableAttributes populates each table object's engine, collation,
// and estimated row count from information_schema.tables.
func (d *mysqlDriver) attachMySQLTableAttributes(ctx context.Context, objs []metadata.Object, pairs string, args []any) error {
	q := `
SELECT table_schema, table_name, engine, table_collation, table_rows
FROM information_schema.tables
WHERE (table_schema, table_name) IN (` + pairs + `)`
	rows, err := d.db.QueryContext(ctx, q, args...)
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

// InspectDefinition serves one object's canonical text definition on demand via
// SHOW CREATE, so bulk InspectObjects (and every schema snapshot) skips the
// per-object SHOW CREATE TABLE round trip and the routine-body column it used to
// carry. Tables yield a "DDL" descriptor; views and routines yield "Definition".
// Unsupported kinds (e.g. triggers), or an object that no longer exists, yield a
// nil descriptor with a nil error.
func (d *mysqlDriver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
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

	// MySQL cannot bind identifiers; mysqlQuoteQualified escapes both components.
	// codeql[go/sql-injection]
	rows, err := d.db.QueryContext(ctx, stmt+mysqlQuoteQualified(ref.Scope.Name("database"), ref.Name))
	if err != nil {
		// A dropped object, or an account without privileges on it, is reported
		// as "not available" rather than a hard error, matching the other
		// engines' lazy-definition contract.
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
	return mysqlSourceDescriptor(title, cells[createIdx].String), nil
}

func mysqlQuoteQualified(namespace, name string) string {
	return mysqlQuoteIdent(namespace) + "." + mysqlQuoteIdent(name)
}

func mysqlQuoteIdent(s string) string {
	return "`" + strings.ReplaceAll(s, "`", "``") + "`"
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

func mysqlSourceDescriptor(title, body string) *metadata.Descriptor {
	if body == "" {
		return nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: "sql", Body: body},
	}
}

func (d *mysqlDriver) inspectRoutines(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
	rows, err := d.db.QueryContext(ctx, q, args...)
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

func (d *mysqlDriver) inspectTriggers(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT trigger_schema, trigger_name, action_timing, event_manipulation,
       event_object_schema, event_object_table, action_statement
FROM information_schema.triggers
WHERE (trigger_schema, trigger_name) IN (` + pairs + `)
ORDER BY trigger_schema, trigger_name`
	rows, err := d.db.QueryContext(ctx, q, args...)
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
