package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/driver"
	build "github.com/sqlwarden/internal/driver/internal/build"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	driver.Register("mysql", func() driver.Driver { return &mysqlDriver{} })
}

type mysqlDriver struct {
	db          *sql.DB
	scanOptions driver.ScanOptions
}

// ensureParams ensures parseTime=true is in the DSN.
func ensureParams(dsn string) string {
	params := map[string]string{
		"parseTime": "true",
	}

	// Split DSN into base and query string parts.
	// MySQL DSN format: [user[:password]@][net[(addr)]]/dbname[?param1=value1&...]
	sep := strings.LastIndex(dsn, "?")
	var base, query string
	if sep == -1 {
		base = dsn
		query = ""
	} else {
		base = dsn[:sep]
		query = dsn[sep+1:]
	}

	existing := map[string]bool{}
	if query != "" {
		for part := range strings.SplitSeq(query, "&") {
			if kv := strings.SplitN(part, "=", 2); len(kv) == 2 {
				existing[kv[0]] = true
			}
		}
	}

	var extra []string
	for k, v := range params {
		if !existing[k] {
			extra = append(extra, k+"="+v)
		}
	}

	if len(extra) == 0 {
		return dsn
	}

	addition := strings.Join(extra, "&")
	if query == "" {
		return base + "?" + addition
	}
	return base + "?" + query + "&" + addition
}

func (d *mysqlDriver) Connect(ctx context.Context, cfg driver.ConnectionConfig) error {
	dsn := ensureParams(cfg.DSN)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mysql: ping: %w", err)
	}
	d.db = db
	d.scanOptions = driver.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	return nil
}

func (d *mysqlDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *mysqlDriver) Close() error {
	return d.db.Close()
}

func (d *mysqlDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: query: %w", err)
	}
	return driver.ScanRows(rows, d.scanOptions)
}

func (d *mysqlDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: execute: %w", err)
	}
	return driver.ScanRows(rows, d.scanOptions)
}

func (d *mysqlDriver) StartQuery(ctx context.Context, req driver.QueryRequest) (driver.QueryCursor, error) {
	rows, err := d.db.QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: start query: %w", err)
	}
	cursor, err := driver.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("mysql: start query cursor: %w", err)
	}
	return cursor, nil
}

func (d *mysqlDriver) Dialect() driver.Dialect {
	return driver.DialectMySQL
}

func (d *mysqlDriver) SchemaSpec() schema.SchemaSpec {
	return schema.SchemaSpec{
		Dialect: "mysql",
		Kinds: []schema.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "procedure", Label: "Procedure", PluralLabel: "Procedures", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "trigger", Label: "Trigger", PluralLabel: "Triggers", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

func (d *mysqlDriver) InspectCatalog(ctx context.Context, opts schema.CatalogOptions) (*schema.Catalog, error) {
	database := opts.Database
	if database == "" {
		if err := d.db.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
			return nil, fmt.Errorf("mysql: catalog database name: %w", err)
		}
	}

	b := build.NewCatalog()
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
		b.AddRef(ns, kind, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: catalog tables rows: %w", err)
	}
	rows.Close()

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
		b.AddRef(ns, kind, name)
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
		b.AddRef(ns, "trigger", name)
	}
	if err := triggerRows.Err(); err != nil {
		triggerRows.Close()
		return nil, fmt.Errorf("mysql: catalog triggers rows: %w", err)
	}
	triggerRows.Close()

	return b.Build("", "mysql", database), nil
}

func (d *mysqlDriver) InspectObjects(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	var relRefs []schema.ObjectRef
	var routineRefs []schema.ObjectRef
	var triggerRefs []schema.ObjectRef
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

	var out []schema.Object
	if len(relRefs) > 0 {
		objs, err := d.introspectRelational(ctx, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(routineRefs) > 0 {
		objs, err := d.introspectRoutines(ctx, routineRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(triggerRefs) > 0 {
		objs, err := d.introspectTriggers(ctx, triggerRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

func mysqlPairFilter(refs []schema.ObjectRef) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, len(refs)*2)
	for i, ref := range refs {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?)")
		args = append(args, ref.Namespace, ref.Name)
	}
	return sb.String(), args
}

func (d *mysqlDriver) introspectRelational(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	kindOf := make(map[string]string, len(refs))
	for _, ref := range refs {
		kindOf[ref.Namespace+"\x00"+ref.Name] = ref.Kind
	}
	refFor := func(ns, name string) schema.ObjectRef {
		return schema.ObjectRef{Namespace: ns, Kind: kindOf[ns+"\x00"+name], Name: name}
	}

	b := build.NewRelational()
	for _, ref := range refs {
		b.Ensure(ref)
	}

	pairs, args := mysqlPairFilter(refs)

	colQ := `
SELECT table_schema, table_name, column_name, column_type, is_nullable, column_default, ordinal_position
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
		if err := crows.Scan(&ns, &tbl, &col, &dtype, &nullable, &def, &ord); err != nil {
			crows.Close()
			return nil, fmt.Errorf("mysql: object columns scan: %w", err)
		}
		c := schema.Column{Name: col, DataType: dtype, Nullable: nullable == "YES", Ordinal: ord}
		if def.Valid {
			v := def.String
			c.Default = &v
		}
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
			schema.ObjectRef{Namespace: refNs, Kind: "table", Name: refTbl}, refCol)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("mysql: object fk rows: %w", err)
	}
	frows.Close()

	idxQ := `
SELECT table_schema, table_name, index_name, non_unique, column_name, seq_in_index
FROM information_schema.statistics
WHERE (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, index_name, seq_in_index`
	irows, err := d.db.QueryContext(ctx, idxQ, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: object indexes: %w", err)
	}
	type idxKey struct{ ns, tbl, name string }
	indexes := map[idxKey]*schema.Index{}
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
			ix = &schema.Index{Name: name, Unique: nonUnique == 0}
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

	return b.Build(), nil
}

func (d *mysqlDriver) introspectRoutines(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	kindOf := make(map[string]string, len(refs))
	for _, ref := range refs {
		kindOf[ref.Namespace+"\x00"+ref.Name] = ref.Kind
	}
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT routine_schema, routine_name, routine_type, data_type, routine_definition,
       external_language, sql_data_access, is_deterministic
FROM information_schema.routines
WHERE (routine_schema, routine_name) IN (` + pairs + `)
ORDER BY routine_schema, routine_name`
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: routine detail: %w", err)
	}
	defer rows.Close()

	var out []schema.Object
	for rows.Next() {
		var ns, name, routineType, sqlAccess, deterministic string
		var dataType, definition, language sql.NullString
		if err := rows.Scan(&ns, &name, &routineType, &dataType, &definition, &language, &sqlAccess, &deterministic); err != nil {
			return nil, fmt.Errorf("mysql: routine detail scan: %w", err)
		}
		kind := kindOf[ns+"\x00"+name]
		if kind == "" {
			kind = strings.ToLower(routineType)
		}
		fields := []schema.Field{
			{Name: "Type", Value: routineType},
			{Name: "SQL data access", Value: sqlAccess},
			{Name: "Deterministic", Value: deterministic},
		}
		if dataType.Valid && dataType.String != "" {
			fields = append(fields, schema.Field{Name: "Returns", Value: dataType.String})
		}
		if language.Valid && language.String != "" {
			fields = append(fields, schema.Field{Name: "Language", Value: language.String})
		}
		obj := schema.Object{
			Ref: schema.ObjectRef{Namespace: ns, Kind: kind, Name: name},
			Descriptors: []schema.Descriptor{
				{Kind: "fields", Title: "Routine", Fields: fields},
			},
		}
		if definition.Valid && definition.String != "" {
			obj.Descriptors = append(obj.Descriptors, schema.Descriptor{
				Kind:   "source",
				Title:  "Definition",
				Source: &schema.Source{Language: "sql", Body: definition.String},
			})
		}
		out = append(out, obj)
	}
	return out, rows.Err()
}

func (d *mysqlDriver) introspectTriggers(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
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

	var out []schema.Object
	for rows.Next() {
		var ns, name, timing, event, tableNs, tableName, statement string
		if err := rows.Scan(&ns, &name, &timing, &event, &tableNs, &tableName, &statement); err != nil {
			return nil, fmt.Errorf("mysql: trigger detail scan: %w", err)
		}
		out = append(out, schema.Object{
			Ref: schema.ObjectRef{Namespace: ns, Kind: "trigger", Name: name},
			Descriptors: []schema.Descriptor{
				{Kind: "fields", Title: "Trigger", Fields: []schema.Field{
					{Name: "Timing", Value: timing},
					{Name: "Event", Value: event},
					{Name: "Table", Value: tableNs + "." + tableName},
				}},
				{Kind: "source", Title: "Statement", Source: &schema.Source{Language: "sql", Body: statement}},
			},
		})
	}
	return out, rows.Err()
}
