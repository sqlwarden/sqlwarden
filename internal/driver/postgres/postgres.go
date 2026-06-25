package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/dbengine/dbsql"
	"github.com/sqlwarden/internal/dbengine/schema"
	build "github.com/sqlwarden/internal/dbengine/schema/build"
	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func init() {
	driver.Register("postgres", func() driver.Driver { return &postgresDriver{} })
}

type postgresDriver struct {
	db          *sql.DB
	scanOptions dbsql.ScanOptions
}

func (d *postgresDriver) Connect(ctx context.Context, cfg driver.ConnectionConfig) error {
	db, err := sql.Open("pgx", cfg.DSN)
	if err != nil {
		return fmt.Errorf("postgres: open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	d.db = db
	d.scanOptions = dbsql.ScanOptions{MaxRows: cfg.MaxResultRows, MaxBytes: cfg.MaxResultBytes}
	return nil
}

func (d *postgresDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *postgresDriver) Close() error {
	return d.db.Close()
}

func (d *postgresDriver) Query(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: query: %w", err)
	}
	return dbsql.ScanRows(rows, d.scanOptions)
}

func (d *postgresDriver) Execute(ctx context.Context, query string, args ...any) (*result.ResultSet, error) {
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: execute: %w", err)
	}
	return dbsql.ScanRows(rows, d.scanOptions)
}

func (d *postgresDriver) StartQuery(ctx context.Context, req dbsql.QueryRequest) (dbsql.QueryCursor, error) {
	rows, err := d.db.QueryContext(ctx, req.SQL, req.Args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: start query: %w", err)
	}
	cursor, err := dbsql.NewSQLRowsCursor(rows)
	if err != nil {
		return nil, fmt.Errorf("postgres: start query cursor: %w", err)
	}
	return cursor, nil
}

func (d *postgresDriver) Dialect() driver.Dialect {
	return driver.DialectPostgres
}

func (d *postgresDriver) SchemaSpec() schema.SchemaSpec {
	return schema.SchemaSpec{
		Dialect: "postgres",
		Kinds: []schema.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "materialized_view", Label: "Materialized View", PluralLabel: "Materialized Views", Order: 3, Relational: true, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

func (d *postgresDriver) InspectCatalog(ctx context.Context, opts schema.CatalogOptions) (*schema.Catalog, error) {
	b := build.NewCatalog()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("materialized_view")
	b.DeclareKind("function")
	b.DeclareKind("sequence")

	const tblQ = `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_catalog = current_database()
  AND table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name`
	if err := d.queryRefs(ctx, tblQ, func(ns, name, t string) {
		kind := "table"
		if t == "VIEW" {
			kind = "view"
		}
		b.AddRef(ns, kind, name)
	}); err != nil {
		return nil, fmt.Errorf("postgres: catalog tables: %w", err)
	}

	const mvQ = `
SELECT n.nspname, c.relname
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'm' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname`
	if err := d.queryRefs(ctx, mvQ, func(ns, name, _ string) { b.AddRef(ns, "materialized_view", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog matviews: %w", err)
	}

	const fnQ = `
SELECT n.nspname, p.proname
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = 'f' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, p.proname`
	if err := d.queryRefs(ctx, fnQ, func(ns, name, _ string) { b.AddRef(ns, "function", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog functions: %w", err)
	}

	const seqQ = `
SELECT sequence_schema, sequence_name
FROM information_schema.sequences
WHERE sequence_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY sequence_schema, sequence_name`
	if err := d.queryRefs(ctx, seqQ, func(ns, name, _ string) { b.AddRef(ns, "sequence", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog sequences: %w", err)
	}

	var dbName string
	if err := d.db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&dbName); err != nil {
		return nil, fmt.Errorf("postgres: catalog database name: %w", err)
	}
	return b.Build("", "postgres", dbName), nil
}

// queryRefs runs a 2- or 3-column query (schema, name[, type]) and calls fn per
// row; the third column is passed as "" when the query selects only two columns.
func (d *postgresDriver) queryRefs(ctx context.Context, q string, fn func(ns, name, extra string)) error {
	rows, err := d.db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	three := len(cols) == 3
	for rows.Next() {
		var ns, name, extra string
		if three {
			if err := rows.Scan(&ns, &name, &extra); err != nil {
				return err
			}
		} else {
			if err := rows.Scan(&ns, &name); err != nil {
				return err
			}
		}
		fn(ns, name, extra)
	}
	return rows.Err()
}

func (d *postgresDriver) InspectObjects(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	var relRefs, mvRefs, fnRefs, seqRefs []schema.ObjectRef
	for _, r := range refs {
		switch r.Kind {
		case "table", "view":
			relRefs = append(relRefs, r)
		case "materialized_view":
			mvRefs = append(mvRefs, r)
		case "function":
			fnRefs = append(fnRefs, r)
		case "sequence":
			seqRefs = append(seqRefs, r)
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
	if len(mvRefs) > 0 {
		objs, err := d.introspectMatviews(ctx, mvRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(fnRefs) > 0 {
		objs, err := d.introspectFunctions(ctx, fnRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(seqRefs) > 0 {
		objs, err := d.introspectSequences(ctx, seqRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// pairFilter builds a "($n,$n+1),($n+2,$n+3),…" tuple list plus the flattened
// (namespace, name) args, for a "(schema, name) IN (...)" predicate.
func pairFilter(refs []schema.ObjectRef, start int) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, len(refs)*2)
	for i, r := range refs {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "($%d,$%d)", start+i*2, start+i*2+1)
		args = append(args, r.Namespace, r.Name)
	}
	return sb.String(), args
}

func (d *postgresDriver) introspectRelational(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	kindOf := make(map[string]string, len(refs))
	for _, r := range refs {
		kindOf[r.Namespace+"\x00"+r.Name] = r.Kind
	}
	refFor := func(ns, name string) schema.ObjectRef {
		return schema.ObjectRef{Namespace: ns, Kind: kindOf[ns+"\x00"+name], Name: name}
	}

	b := build.NewRelational()
	for _, r := range refs {
		b.Ensure(r)
	}

	pairs, args := pairFilter(refs, 1)

	colQ := `
SELECT table_schema, table_name, column_name, udt_name, is_nullable, column_default, ordinal_position
FROM information_schema.columns
WHERE table_catalog = current_database()
  AND (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, ordinal_position`
	crows, err := d.db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: object columns: %w", err)
	}
	for crows.Next() {
		var ns, tbl, col, dtype, nullable string
		var def sql.NullString
		var ord int
		if err := crows.Scan(&ns, &tbl, &col, &dtype, &nullable, &def, &ord); err != nil {
			crows.Close()
			return nil, fmt.Errorf("postgres: object columns scan: %w", err)
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
		return nil, fmt.Errorf("postgres: object columns rows: %w", err)
	}
	crows.Close()

	pkQ := `
SELECT tc.table_schema, tc.table_name, kcu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'PRIMARY KEY'
  AND (tc.table_schema, tc.table_name) IN (` + pairs + `)
ORDER BY tc.table_schema, tc.table_name, kcu.ordinal_position`
	prows, err := d.db.QueryContext(ctx, pkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: object pk: %w", err)
	}
	for prows.Next() {
		var ns, tbl, col string
		if err := prows.Scan(&ns, &tbl, &col); err != nil {
			prows.Close()
			return nil, fmt.Errorf("postgres: object pk scan: %w", err)
		}
		b.AddPrimaryKeyColumn(refFor(ns, tbl), col)
	}
	if err := prows.Err(); err != nil {
		prows.Close()
		return nil, fmt.Errorf("postgres: object pk rows: %w", err)
	}
	prows.Close()

	// ref_schema is the cross-schema fix: foreign keys carry a qualified target.
	fkQ := `
SELECT tc.table_schema, tc.table_name, tc.constraint_name, kcu.column_name,
       ccu.table_schema AS ref_schema, ccu.table_name AS ref_table, ccu.column_name AS ref_column
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY'
  AND (tc.table_schema, tc.table_name) IN (` + pairs + `)
ORDER BY tc.table_schema, tc.table_name, tc.constraint_name, kcu.ordinal_position`
	frows, err := d.db.QueryContext(ctx, fkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: object fk: %w", err)
	}
	for frows.Next() {
		var ns, tbl, name, col, refNs, refTbl, refCol string
		if err := frows.Scan(&ns, &tbl, &name, &col, &refNs, &refTbl, &refCol); err != nil {
			frows.Close()
			return nil, fmt.Errorf("postgres: object fk scan: %w", err)
		}
		b.AddForeignKeyColumn(refFor(ns, tbl), name, col,
			schema.ObjectRef{Namespace: refNs, Kind: "table", Name: refTbl}, refCol)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("postgres: object fk rows: %w", err)
	}
	frows.Close()

	idxQ := `
SELECT schemaname, tablename, indexname, indexdef
FROM pg_indexes
WHERE (schemaname, tablename) IN (` + pairs + `)
ORDER BY schemaname, tablename, indexname`
	irows, err := d.db.QueryContext(ctx, idxQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: object indexes: %w", err)
	}
	for irows.Next() {
		var ns, tbl, name, def string
		if err := irows.Scan(&ns, &tbl, &name, &def); err != nil {
			irows.Close()
			return nil, fmt.Errorf("postgres: object index scan: %w", err)
		}
		unique := strings.Contains(def, "CREATE UNIQUE INDEX")
		b.AddIndex(refFor(ns, tbl), schema.Index{Name: name, Unique: unique, Attributes: map[string]any{"definition": def}})
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return nil, fmt.Errorf("postgres: object index rows: %w", err)
	}
	irows.Close()

	return b.Build(), nil
}

func (d *postgresDriver) introspectMatviews(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	b := build.NewRelational()
	for _, r := range refs {
		b.Ensure(r)
	}
	refFor := func(ns, name string) schema.ObjectRef {
		return schema.ObjectRef{Namespace: ns, Kind: "materialized_view", Name: name}
	}
	pairs, args := pairFilter(refs, 1)

	colQ := `
SELECT n.nspname, c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnum
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE c.relkind = 'm' AND (n.nspname, c.relname) IN (` + pairs + `)
ORDER BY n.nspname, c.relname, a.attnum`
	rows, err := d.db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: matview columns: %w", err)
	}
	for rows.Next() {
		var ns, mv, col, dtype string
		var attnum int
		if err := rows.Scan(&ns, &mv, &col, &dtype, &attnum); err != nil {
			rows.Close()
			return nil, fmt.Errorf("postgres: matview columns scan: %w", err)
		}
		b.AddColumn(refFor(ns, mv), schema.Column{Name: col, DataType: dtype, Ordinal: attnum})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("postgres: matview columns rows: %w", err)
	}
	rows.Close()
	return b.Build(), nil
}

func (d *postgresDriver) introspectFunctions(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT n.nspname, p.proname,
       pg_get_function_arguments(p.oid),
       pg_get_function_result(p.oid),
       l.lanname,
       pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
JOIN pg_language l ON l.oid = p.prolang
WHERE p.prokind = 'f' AND (n.nspname, p.proname) IN (` + pairs + `)
ORDER BY n.nspname, p.proname`
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: function detail: %w", err)
	}
	defer rows.Close()
	var out []schema.Object
	for rows.Next() {
		var ns, name, fnArgs, lang, def string
		var ret sql.NullString
		if err := rows.Scan(&ns, &name, &fnArgs, &ret, &lang, &def); err != nil {
			return nil, fmt.Errorf("postgres: function detail scan: %w", err)
		}
		fields := []schema.Field{
			{Name: "Arguments", Value: fnArgs},
			{Name: "Language", Value: lang},
		}
		if ret.Valid {
			fields = append(fields, schema.Field{Name: "Returns", Value: ret.String})
		}
		out = append(out, schema.Object{
			Ref: schema.ObjectRef{Namespace: ns, Kind: "function", Name: name},
			Descriptors: []schema.Descriptor{
				{Kind: "fields", Title: "Signature", Fields: fields},
				{Kind: "source", Title: "Definition", Source: &schema.Source{Language: lang, Body: def}},
			},
		})
	}
	return out, rows.Err()
}

func (d *postgresDriver) introspectSequences(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT sequence_schema, sequence_name, data_type
FROM information_schema.sequences
WHERE (sequence_schema, sequence_name) IN (` + pairs + `)
ORDER BY sequence_schema, sequence_name`
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: sequence detail: %w", err)
	}
	defer rows.Close()
	var out []schema.Object
	for rows.Next() {
		var ns, name, dtype string
		if err := rows.Scan(&ns, &name, &dtype); err != nil {
			return nil, fmt.Errorf("postgres: sequence detail scan: %w", err)
		}
		out = append(out, schema.Object{
			Ref: schema.ObjectRef{Namespace: ns, Kind: "sequence", Name: name},
			Descriptors: []schema.Descriptor{
				{Kind: "fields", Title: "Sequence", Fields: []schema.Field{{Name: "Data type", Value: dtype}}},
			},
		})
	}
	return out, rows.Err()
}
