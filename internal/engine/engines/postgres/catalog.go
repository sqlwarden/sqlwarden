package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

// CatalogTables enumerates every table and view visible to the current
// database, invoking add once per object with its schema, name, and resolved
// kind ("table" or "view").
func CatalogTables(ctx context.Context, db *sql.DB, add func(schema, name, kind string)) error {
	const q = `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_catalog = current_database()
  AND table_schema NOT IN ('pg_catalog', 'information_schema')
  AND table_type <> 'FOREIGN'
ORDER BY table_schema, table_name`
	return queryRefs(ctx, db, q, func(ns, name, t string) {
		kind := "table"
		if t == "VIEW" {
			kind = "view"
		}
		add(ns, name, kind)
	})
}

// CatalogMaterializedViews enumerates every materialized view visible to the
// current database.
func CatalogMaterializedViews(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT n.nspname, c.relname
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'm' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// CatalogFunctions enumerates every plain function visible to the current
// database (procedures and aggregates are excluded via prokind = 'f'). A
// schema+name pair is emitted once even when Postgres overloads it with
// multiple signatures; FunctionObjects and FunctionDefinition surface every
// overload under that single entry.
func CatalogFunctions(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT DISTINCT n.nspname, p.proname
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = 'f' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, p.proname`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// CatalogSequences enumerates every sequence visible to the current database.
func CatalogSequences(ctx context.Context, db *sql.DB, add func(schema, name string)) error {
	const q = `
SELECT sequence_schema, sequence_name
FROM information_schema.sequences
WHERE sequence_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY sequence_schema, sequence_name`
	return queryRefs(ctx, db, q, func(ns, name, _ string) { add(ns, name) })
}

// AttachRowCounts reports the approximate row count (pg_class.reltuples) for
// every table and materialized view. reltuples is a planner statistic
// refreshed by ANALYZE/autovacuum, not a live COUNT(*), which is what keeps
// this query cheap regardless of table size.
func AttachRowCounts(ctx context.Context, db *sql.DB, set func(schema, kind, name string, count int64)) error {
	const q = `
SELECT n.nspname, c.relname, c.relkind, c.reltuples
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'm') AND n.nspname NOT IN ('pg_catalog', 'information_schema')`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ns, name, relkind string
		var reltuples float64
		if err := rows.Scan(&ns, &name, &relkind, &reltuples); err != nil {
			return err
		}
		if reltuples < 0 {
			continue
		}
		kind := "table"
		if relkind == "m" {
			kind = "materialized_view"
		}
		set(ns, kind, name, int64(reltuples))
	}
	return rows.Err()
}

// queryRefs runs a 2- or 3-column query (schema, name[, type]) and calls fn per
// row; the third column is passed as "" when the query selects only two columns.
func queryRefs(ctx context.Context, db *sql.DB, q string, fn func(ns, name, extra string)) error {
	rows, err := db.QueryContext(ctx, q)
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

// pairFilter builds a "($n,$n+1),($n+2,$n+3),…" tuple list plus the flattened
// (namespace, name) args, for a "(schema, name) IN (...)" predicate.
func pairFilter(refs []metadata.ObjectRef, start int) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, len(refs)*2)
	for i, r := range refs {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, "($%d,$%d)", start+i*2, start+i*2+1)
		args = append(args, r.Scope.Name("schema"), r.Name)
	}
	return sb.String(), args
}

// RelationalObjects fetches full column/PK/FK/index detail for tables and
// views named in refs.
func RelationalObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	refByName := make(map[string]metadata.ObjectRef, len(refs))
	for _, r := range refs {
		refByName[r.Scope.Name("schema")+"\x00"+r.Name] = r
	}
	refFor := func(ns, name string) metadata.ObjectRef {
		return refByName[ns+"\x00"+name]
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
	crows, err := db.QueryContext(ctx, colQ, args...)
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
		c := metadata.Column{Name: col, DataType: dtype, Nullable: nullable == "YES", Ordinal: ord}
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
	prows, err := db.QueryContext(ctx, pkQ, args...)
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
	frows, err := db.QueryContext(ctx, fkQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: object fk: %w", err)
	}
	for frows.Next() {
		var ns, tbl, name, col, refNs, refTbl, refCol string
		if err := frows.Scan(&ns, &tbl, &name, &col, &refNs, &refTbl, &refCol); err != nil {
			frows.Close()
			return nil, fmt.Errorf("postgres: object fk scan: %w", err)
		}
		source := refFor(ns, tbl)
		b.AddForeignKeyColumn(source, name, col,
			metadata.ObjectRef{Scope: source.Scope.With("schema", refNs), Kind: "table", Name: refTbl}, refCol)
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, fmt.Errorf("postgres: object fk rows: %w", err)
	}
	frows.Close()

	// One row per index key column: pg_get_indexdef(idx, n, true) renders the
	// n-th key as a column name or expression, so this also covers expression
	// indexes. Columns are aggregated per index in attnum order.
	idxQ := `
SELECT ns.nspname, t.relname, i.relname, ix.indisunique,
       pg_get_indexdef(i.oid),
       pg_get_indexdef(i.oid, g.n, true)
FROM pg_index ix
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace ns ON ns.oid = t.relnamespace
CROSS JOIN LATERAL generate_series(1, ix.indnkeyatts) AS g(n)
WHERE (ns.nspname, t.relname) IN (` + pairs + `)
  AND NOT ix.indisprimary
  AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conindid = ix.indexrelid)
ORDER BY ns.nspname, t.relname, i.relname, g.n`
	irows, err := db.QueryContext(ctx, idxQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: object indexes: %w", err)
	}
	type idxKey struct{ ns, tbl, name string }
	indexes := map[idxKey]*metadata.SecondaryIndex{}
	var indexOrder []idxKey
	for irows.Next() {
		var ns, tbl, name, def, col string
		var unique bool
		if err := irows.Scan(&ns, &tbl, &name, &unique, &def, &col); err != nil {
			irows.Close()
			return nil, fmt.Errorf("postgres: object index scan: %w", err)
		}
		key := idxKey{ns: ns, tbl: tbl, name: name}
		ix, ok := indexes[key]
		if !ok {
			ix = &metadata.SecondaryIndex{Name: name, Unique: unique, Attributes: map[string]any{"definition": def}}
			indexes[key] = ix
			indexOrder = append(indexOrder, key)
		}
		ix.Columns = append(ix.Columns, col)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return nil, fmt.Errorf("postgres: object index rows: %w", err)
	}
	irows.Close()
	for _, key := range indexOrder {
		b.AddIndex(refFor(key.ns, key.tbl), *indexes[key])
	}

	out := b.Build()
	if err := attachPostgresComments(ctx, db, out, pairs, args); err != nil {
		return nil, err
	}
	if err := attachPostgresPartitions(ctx, db, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachPostgresComments populates table and column "comment" attributes from
// obj_description / col_description.
func attachPostgresComments(ctx context.Context, db *sql.DB, objs []metadata.Object, pairs string, args []any) error {
	tableComments := map[string]string{}
	trows, err := db.QueryContext(ctx, `
SELECT n.nspname, c.relname, obj_description(c.oid)
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE (n.nspname, c.relname) IN (`+pairs+`)`, args...)
	if err != nil {
		return fmt.Errorf("postgres: table comments: %w", err)
	}
	for trows.Next() {
		var ns, name string
		var comment sql.NullString
		if err := trows.Scan(&ns, &name, &comment); err != nil {
			trows.Close()
			return fmt.Errorf("postgres: table comments scan: %w", err)
		}
		if comment.Valid && comment.String != "" {
			tableComments[ns+"\x00"+name] = comment.String
		}
	}
	if err := trows.Err(); err != nil {
		trows.Close()
		return fmt.Errorf("postgres: table comments rows: %w", err)
	}
	trows.Close()

	type colKey struct{ ns, tbl, col string }
	colComments := map[colKey]string{}
	crows, err := db.QueryContext(ctx, `
SELECT n.nspname, c.relname, a.attname, col_description(c.oid, a.attnum)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE (n.nspname, c.relname) IN (`+pairs+`)`, args...)
	if err != nil {
		return fmt.Errorf("postgres: column comments: %w", err)
	}
	for crows.Next() {
		var ns, tbl, col string
		var comment sql.NullString
		if err := crows.Scan(&ns, &tbl, &col, &comment); err != nil {
			crows.Close()
			return fmt.Errorf("postgres: column comments scan: %w", err)
		}
		if comment.Valid && comment.String != "" {
			colComments[colKey{ns: ns, tbl: tbl, col: col}] = comment.String
		}
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return fmt.Errorf("postgres: column comments rows: %w", err)
	}
	crows.Close()

	for i := range objs {
		ns, name := objs[i].Ref.Scope.Name("schema"), objs[i].Ref.Name
		if c := tableComments[ns+"\x00"+name]; c != "" {
			setObjectAttr(&objs[i], "comment", c)
		}
		if objs[i].Relational == nil {
			continue
		}
		for j := range objs[i].Relational.Columns {
			col := &objs[i].Relational.Columns[j]
			if c := colComments[colKey{ns: ns, tbl: name, col: col.Name}]; c != "" {
				setColumnAttr(col, "comment", c)
			}
		}
	}
	return nil
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

func setColumnAttr(c *metadata.Column, key, value string) {
	if value == "" {
		return
	}
	if c.Attributes == nil {
		c.Attributes = map[string]any{}
	}
	c.Attributes[key] = value
}

// MaterializedViewObjects fetches column detail for materialized views named
// in refs.
func MaterializedViewObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	b := build.NewRelational()
	for _, r := range refs {
		b.Ensure(r)
	}
	refFor := func(ns, name string) metadata.ObjectRef {
		return postgresRequestedRef(refs, ns, name, "materialized_view")
	}
	pairs, args := pairFilter(refs, 1)

	colQ := `
SELECT n.nspname, c.relname, a.attname, format_type(a.atttypid, a.atttypmod), a.attnum
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
WHERE c.relkind = 'm' AND (n.nspname, c.relname) IN (` + pairs + `)
ORDER BY n.nspname, c.relname, a.attnum`
	rows, err := db.QueryContext(ctx, colQ, args...)
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
		b.AddColumn(refFor(ns, mv), metadata.Column{Name: col, DataType: dtype, Ordinal: attnum})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("postgres: matview columns rows: %w", err)
	}
	rows.Close()
	return b.Build(), nil
}

// FunctionObjects fetches signature detail for functions named in refs. A
// schema+name pair may resolve to multiple overloads (ObjectRef identity is
// schema+name only, matching CatalogFunctions); every overload is emitted as
// its own "Signature" descriptor on the one Object for that name.
func FunctionObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT n.nspname, p.proname,
       pg_get_function_arguments(p.oid),
       pg_get_function_result(p.oid),
       l.lanname
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
JOIN pg_language l ON l.oid = p.prolang
WHERE p.prokind = 'f' AND (n.nspname, p.proname) IN (` + pairs + `)
ORDER BY n.nspname, p.proname, p.oid`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: function detail: %w", err)
	}
	defer rows.Close()

	var out []metadata.Object
	var ns, name string
	var overloads [][]metadata.Field // one entry per overload of the current (ns, name)
	flush := func() {
		if len(overloads) == 0 {
			return
		}
		descriptors := make([]metadata.Descriptor, 0, len(overloads))
		for i, fields := range overloads {
			title := "Signature"
			if len(overloads) > 1 {
				title = fmt.Sprintf("Signature (overload %d of %d)", i+1, len(overloads))
			}
			descriptors = append(descriptors, metadata.Descriptor{Kind: "fields", Title: title, Fields: fields})
		}
		out = append(out, metadata.Object{
			Ref:         postgresRequestedRef(refs, ns, name, "function"),
			Descriptors: descriptors,
		})
		overloads = nil
	}
	for rows.Next() {
		var rowNS, rowName, fnArgs, lang string
		var ret sql.NullString
		if err := rows.Scan(&rowNS, &rowName, &fnArgs, &ret, &lang); err != nil {
			return nil, fmt.Errorf("postgres: function detail scan: %w", err)
		}
		if rowNS != ns || rowName != name {
			flush()
			ns, name = rowNS, rowName
		}
		fields := []metadata.Field{
			{Name: "Arguments", Value: fnArgs},
			{Name: "Language", Value: lang},
		}
		if ret.Valid {
			fields = append(fields, metadata.Field{Name: "Returns", Value: ret.String})
		}
		overloads = append(overloads, fields)
	}
	flush()
	return out, rows.Err()
}

// SequenceObjects fetches data-type detail for sequences named in refs.
func SequenceObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT sequence_schema, sequence_name, data_type
FROM information_schema.sequences
WHERE (sequence_schema, sequence_name) IN (` + pairs + `)
ORDER BY sequence_schema, sequence_name`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: sequence detail: %w", err)
	}
	defer rows.Close()
	var out []metadata.Object
	for rows.Next() {
		var ns, name, dtype string
		if err := rows.Scan(&ns, &name, &dtype); err != nil {
			return nil, fmt.Errorf("postgres: sequence detail scan: %w", err)
		}
		out = append(out, metadata.Object{
			Ref: postgresRequestedRef(refs, ns, name, "sequence"),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Sequence", Fields: []metadata.Field{{Name: "Data type", Value: dtype}}},
			},
		})
	}
	return out, rows.Err()
}

func postgresRequestedRef(refs []metadata.ObjectRef, namespace, name, kind string) metadata.ObjectRef {
	for _, ref := range refs {
		if ref.Scope.Name("schema") == namespace && ref.Name == name {
			return ref
		}
	}
	var scope metadata.ScopePath
	if len(refs) > 0 {
		scope = refs[0].Scope.With("schema", namespace)
	}
	return metadata.ObjectRef{Scope: scope, Kind: kind, Name: name}
}

// SourceDescriptor builds a "source" descriptor, or nil if body is empty (a
// dropped/inaccessible object rather than an error).
func SourceDescriptor(title, language, body string) *metadata.Descriptor {
	if body == "" {
		return nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: language, Body: body},
	}
}

// TableDDL reconstructs CREATE TABLE DDL for ref from the catalog: columns
// (types, NOT NULL, defaults, identity), table constraints (PK/UNIQUE/FK/CHECK),
// and secondary indexes. It does not cover non-default identity/sequence
// options (START/INCREMENT), generated/stored columns, partitioning,
// inheritance, storage/WITH params, collations, EXCLUDE constraints, or
// comments — output stays valid SQL, but is not a full pg_dump-fidelity
// reproduction. Returns ("", nil) if ref no longer exists.
func TableDDL(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	args := []any{ref.Scope.Name("schema"), ref.Name}
	relArg := `format('%I.%I', $1::text, $2::text)::regclass`

	var qualified string
	if err := db.QueryRowContext(ctx, `SELECT format('%I.%I', $1::text, $2::text)`, args...).Scan(&qualified); err != nil {
		return "", fmt.Errorf("postgres: ddl qualified name: %w", err)
	}

	// to_regclass yields NULL (not an error) for a relation that no longer
	// exists, so a stale ref resolves to an empty definition rather than the
	// hard error a bare ::regclass cast would raise.
	var exists sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT to_regclass(format('%I.%I', $1::text, $2::text))`, args...).Scan(&exists); err != nil {
		return "", fmt.Errorf("postgres: ddl relation lookup: %w", err)
	}
	if !exists.Valid {
		return "", nil
	}

	var lines []string

	colRows, err := db.QueryContext(ctx, `
SELECT quote_ident(a.attname), format_type(a.atttypid, a.atttypmod), a.attnotnull,
       pg_get_expr(ad.adbin, ad.adrelid), a.attidentity
FROM pg_attribute a
LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
WHERE a.attrelid = `+relArg+` AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, args...)
	if err != nil {
		return "", fmt.Errorf("postgres: ddl columns: %w", err)
	}
	for colRows.Next() {
		var name, typ, identity string
		var notNull bool
		var def sql.NullString
		if err := colRows.Scan(&name, &typ, &notNull, &def, &identity); err != nil {
			colRows.Close()
			return "", fmt.Errorf("postgres: ddl columns scan: %w", err)
		}
		line := "  " + name + " " + typ
		if notNull {
			line += " NOT NULL"
		}
		switch identity {
		case "a":
			line += " GENERATED ALWAYS AS IDENTITY"
		case "d":
			line += " GENERATED BY DEFAULT AS IDENTITY"
		default:
			if def.Valid && def.String != "" {
				line += " DEFAULT " + def.String
			}
		}
		lines = append(lines, line)
	}
	if err := colRows.Err(); err != nil {
		colRows.Close()
		return "", fmt.Errorf("postgres: ddl columns rows: %w", err)
	}
	colRows.Close()
	if len(lines) == 0 {
		return "", nil
	}

	conRows, err := db.QueryContext(ctx, `
SELECT quote_ident(conname), pg_get_constraintdef(oid)
FROM pg_constraint
WHERE conrelid = `+relArg+`
ORDER BY CASE contype WHEN 'p' THEN 0 WHEN 'u' THEN 1 WHEN 'f' THEN 2 WHEN 'c' THEN 3 ELSE 4 END, conname`, args...)
	if err != nil {
		return "", fmt.Errorf("postgres: ddl constraints: %w", err)
	}
	for conRows.Next() {
		var name, def string
		if err := conRows.Scan(&name, &def); err != nil {
			conRows.Close()
			return "", fmt.Errorf("postgres: ddl constraints scan: %w", err)
		}
		lines = append(lines, "  CONSTRAINT "+name+" "+def)
	}
	if err := conRows.Err(); err != nil {
		conRows.Close()
		return "", fmt.Errorf("postgres: ddl constraints rows: %w", err)
	}
	conRows.Close()

	var b strings.Builder
	b.WriteString("CREATE TABLE " + qualified + " (\n" + strings.Join(lines, ",\n") + "\n);")

	idxRows, err := db.QueryContext(ctx, `
SELECT pg_get_indexdef(i.indexrelid)
FROM pg_index i
WHERE i.indrelid = `+relArg+`
  AND NOT i.indisprimary
  AND NOT EXISTS (SELECT 1 FROM pg_constraint con WHERE con.conindid = i.indexrelid)
ORDER BY i.indexrelid::regclass::text`, args...)
	if err != nil {
		return "", fmt.Errorf("postgres: ddl indexes: %w", err)
	}
	for idxRows.Next() {
		var def string
		if err := idxRows.Scan(&def); err != nil {
			idxRows.Close()
			return "", fmt.Errorf("postgres: ddl indexes scan: %w", err)
		}
		b.WriteString("\n\n" + def + ";")
	}
	if err := idxRows.Err(); err != nil {
		idxRows.Close()
		return "", fmt.Errorf("postgres: ddl indexes rows: %w", err)
	}
	idxRows.Close()

	return b.String(), nil
}

// ViewDefinition returns ref's view body via pg_get_viewdef, or ("", nil) if
// ref no longer exists.
func ViewDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	var def sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT pg_get_viewdef(format('%I.%I', $1::text, $2::text)::regclass, true)`,
		ref.Scope.Name("schema"), ref.Name).Scan(&def)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: view definition: %w", err)
	}
	return def.String, nil
}

// FunctionDefinition returns ref's language and body via pg_get_functiondef,
// or ("", "", nil) if ref no longer exists. A schema+name pair may resolve to
// multiple overloads (ObjectRef identity is schema+name only); every
// overload's definition is included, banner-separated when there is more
// than one, so no overload is silently dropped.
func FunctionDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (language, body string, err error) {
	return routineDefinition(ctx, db, ref, 'f', "function")
}

// ProcedureDefinition is FunctionDefinition for prokind = 'p'. Procedures are
// overloadable like functions, so overloads are banner-separated the same way.
func ProcedureDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (language, body string, err error) {
	return routineDefinition(ctx, db, ref, 'p', "procedure")
}

func routineDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef, prokind rune, label string) (language, body string, err error) {
	rows, err := db.QueryContext(ctx, `
SELECT l.lanname, pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
JOIN pg_language l ON l.oid = p.prolang
WHERE p.prokind = $1 AND n.nspname = $2 AND p.proname = $3
ORDER BY p.oid`, string(prokind), ref.Scope.Name("schema"), ref.Name)
	if err != nil {
		return "", "", fmt.Errorf("postgres: %s definition: %w", label, err)
	}
	defer rows.Close()

	var lang string
	var defs []string
	for rows.Next() {
		var rowLang, def sql.NullString
		if err := rows.Scan(&rowLang, &def); err != nil {
			return "", "", fmt.Errorf("postgres: %s definition scan: %w", label, err)
		}
		if lang == "" {
			lang = rowLang.String
		}
		defs = append(defs, def.String)
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("postgres: %s definition: %w", label, err)
	}
	if len(defs) == 0 {
		return "", "", nil
	}
	language = lang
	if language == "" {
		language = "sql"
	}
	if len(defs) == 1 {
		return language, defs[0], nil
	}
	var sb strings.Builder
	for i, def := range defs {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "-- Overload %d of %d\n%s", i+1, len(defs), def)
	}
	return language, sb.String(), nil
}

// MaterializedViewDefinition returns a CREATE MATERIALIZED VIEW statement for
// ref (pg_get_viewdef supplies the query body), or ("", nil) if ref no longer
// exists.
func MaterializedViewDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	var def sql.NullString
	err := db.QueryRowContext(ctx, `
SELECT format(
  E'CREATE MATERIALIZED VIEW %I.%I AS\n%s',
  n.nspname, c.relname, pg_get_viewdef(c.oid, true)
)
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'm' AND n.nspname = $1 AND c.relname = $2`,
		ref.Scope.Name("schema"), ref.Name).Scan(&def)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: materialized view definition: %w", err)
	}
	return def.String, nil
}

// TriggerDefinition returns ref's CREATE TRIGGER statement(s) via
// pg_get_triggerdef, or ("", nil) if ref no longer exists. A trigger name is
// unique per table in Postgres, so a schema+name ref can resolve to several
// triggers; each definition is banner-separated so none is dropped.
func TriggerDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT tbl.relname, pg_get_triggerdef(t.oid, true)
FROM pg_trigger t
JOIN pg_class tbl ON tbl.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = tbl.relnamespace
WHERE NOT t.tgisinternal AND n.nspname = $1 AND t.tgname = $2
ORDER BY tbl.relname`, ref.Scope.Name("schema"), ref.Name)
	if err != nil {
		return "", fmt.Errorf("postgres: trigger definition: %w", err)
	}
	defer rows.Close()

	type entry struct{ table, def string }
	var entries []entry
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.table, &e.def); err != nil {
			return "", fmt.Errorf("postgres: trigger definition scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("postgres: trigger definition: %w", err)
	}
	if len(entries) == 0 {
		return "", nil
	}
	if len(entries) == 1 {
		return entries[0].def + ";", nil
	}
	var sb strings.Builder
	for i, e := range entries {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "-- On %s\n%s;", e.table, e.def)
	}
	return sb.String(), nil
}

// SequenceDefinition reconstructs a CREATE SEQUENCE statement for ref from
// pg_sequences, or ("", nil) if ref no longer exists.
func SequenceDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	var (
		dataType             string
		start, inc, min, max int64
		cache                int64
		cycle                bool
	)
	err := db.QueryRowContext(ctx, `
SELECT data_type::text, start_value, increment_by, min_value, max_value, cache_size, cycle
FROM pg_sequences
WHERE schemaname = $1 AND sequencename = $2`,
		ref.Scope.Name("schema"), ref.Name).
		Scan(&dataType, &start, &inc, &min, &max, &cache, &cycle)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: sequence definition: %w", err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE SEQUENCE %s.%s\n", quoteIdent(ref.Scope.Name("schema")), quoteIdent(ref.Name))
	fmt.Fprintf(&sb, "    AS %s\n", dataType)
	fmt.Fprintf(&sb, "    START WITH %d\n", start)
	fmt.Fprintf(&sb, "    INCREMENT BY %d\n", inc)
	fmt.Fprintf(&sb, "    MINVALUE %d\n", min)
	fmt.Fprintf(&sb, "    MAXVALUE %d\n", max)
	fmt.Fprintf(&sb, "    CACHE %d", cache)
	if cycle {
		sb.WriteString("\n    CYCLE")
	}
	sb.WriteString(";")
	return sb.String(), nil
}

// DomainDefinition reconstructs a CREATE DOMAIN statement for ref from
// pg_type (typtype = 'd') and its pg_constraint rows, or ("", nil) if ref no
// longer exists. Collation and non-default constraint validation state are not
// reproduced; output stays valid SQL.
func DomainDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	var (
		baseType    string
		notNull     bool
		typDefault  sql.NullString
		constraints string
	)
	err := db.QueryRowContext(ctx, `
SELECT format_type(t.typbasetype, t.typtypmod),
       t.typnotnull,
       t.typdefault,
       COALESCE((
         SELECT string_agg('CONSTRAINT ' || quote_ident(con.conname) || ' ' || pg_get_constraintdef(con.oid), E'\n    ' ORDER BY con.conname)
         FROM pg_constraint con WHERE con.contypid = t.oid), '')
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE t.typtype = 'd' AND n.nspname = $1 AND t.typname = $2`,
		ref.Scope.Name("schema"), ref.Name).
		Scan(&baseType, &notNull, &typDefault, &constraints)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: domain definition: %w", err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE DOMAIN %s.%s AS %s", quoteIdent(ref.Scope.Name("schema")), quoteIdent(ref.Name), baseType)
	if typDefault.Valid && typDefault.String != "" {
		fmt.Fprintf(&sb, "\n    DEFAULT %s", typDefault.String)
	}
	if notNull {
		sb.WriteString("\n    NOT NULL")
	}
	if constraints != "" {
		fmt.Fprintf(&sb, "\n    %s", constraints)
	}
	sb.WriteString(";")
	return sb.String(), nil
}

// TypeDefinition reconstructs a CREATE TYPE statement for a composite, enum, or
// range type named by ref, or ("", nil) if ref no longer exists. Composite
// column collations, range subtype opclass/canonical/subtype_diff options, and
// multirange types are not reproduced; output stays valid SQL.
func TypeDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	schema := ref.Scope.Name("schema")
	var (
		typtype string
		oid     uint32
	)
	err := db.QueryRowContext(ctx, `
SELECT t.typtype, t.oid
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE t.typtype IN ('c', 'e', 'r') AND n.nspname = $1 AND t.typname = $2`,
		schema, ref.Name).Scan(&typtype, &oid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: type definition: %w", err)
	}
	header := fmt.Sprintf("CREATE TYPE %s.%s", quoteIdent(schema), quoteIdent(ref.Name))

	switch typtype {
	case "e":
		labels, err := typeDefinitionRows(ctx, db,
			`SELECT quote_literal(enumlabel) FROM pg_enum WHERE enumtypid = $1 ORDER BY enumsortorder`, oid)
		if err != nil {
			return "", err
		}
		return header + " AS ENUM (\n    " + strings.Join(labels, ",\n    ") + "\n);", nil
	case "c":
		cols, err := typeDefinitionRows(ctx, db, `
SELECT quote_ident(a.attname) || ' ' || format_type(a.atttypid, a.atttypmod)
FROM pg_attribute a
WHERE a.attrelid = (SELECT typrelid FROM pg_type WHERE oid = $1)
  AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, oid)
		if err != nil {
			return "", err
		}
		return header + " AS (\n    " + strings.Join(cols, ",\n    ") + "\n);", nil
	default: // "r"
		var subtype string
		if err := db.QueryRowContext(ctx,
			`SELECT format_type(rngsubtype, NULL) FROM pg_range WHERE rngtypid = $1`, oid).
			Scan(&subtype); err != nil {
			return "", fmt.Errorf("postgres: range type definition: %w", err)
		}
		return header + " AS RANGE (\n    SUBTYPE = " + subtype + "\n);", nil
	}
}

func typeDefinitionRows(ctx context.Context, db *sql.DB, q string, oid uint32) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, oid)
	if err != nil {
		return nil, fmt.Errorf("postgres: type definition: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("postgres: type definition scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ForeignTableDefinition reconstructs a CREATE FOREIGN TABLE statement for ref
// from pg_foreign_table, pg_attribute, and pg_foreign_server, or ("", nil) if
// ref no longer exists. Column/table FDW OPTIONS are reproduced;
// generated columns and column collations are not.
func ForeignTableDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (string, error) {
	schema := ref.Scope.Name("schema")
	var (
		server    string
		tableOpts string
		relOID    uint32
	)
	err := db.QueryRowContext(ctx, `
SELECT s.srvname,
       c.oid,
       COALESCE((SELECT string_agg(split_part(opt, '=', 1) || ' ' || quote_literal(substr(opt, strpos(opt, '=') + 1)), ', ')
                 FROM unnest(ft.ftoptions) AS opt), '')
FROM pg_foreign_table ft
JOIN pg_class c ON c.oid = ft.ftrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_foreign_server s ON s.oid = ft.ftserver
WHERE n.nspname = $1 AND c.relname = $2`,
		schema, ref.Name).Scan(&server, &relOID, &tableOpts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("postgres: foreign table definition: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
SELECT quote_ident(a.attname),
       format_type(a.atttypid, a.atttypmod),
       a.attnotnull,
       COALESCE((SELECT string_agg(split_part(opt, '=', 1) || ' ' || quote_literal(substr(opt, strpos(opt, '=') + 1)), ', ')
                 FROM unnest(a.attfdwoptions) AS opt), '')
FROM pg_attribute a
WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum`, relOID)
	if err != nil {
		return "", fmt.Errorf("postgres: foreign table columns: %w", err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name, dataType, colOpts string
		var notNull bool
		if err := rows.Scan(&name, &dataType, &notNull, &colOpts); err != nil {
			return "", fmt.Errorf("postgres: foreign table columns scan: %w", err)
		}
		line := name + " " + dataType
		if notNull {
			line += " NOT NULL"
		}
		if colOpts != "" {
			line += " OPTIONS (" + colOpts + ")"
		}
		cols = append(cols, line)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("postgres: foreign table columns rows: %w", err)
	}
	if len(cols) == 0 {
		return "", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "CREATE FOREIGN TABLE %s.%s (\n    %s\n)\nSERVER %s",
		quoteIdent(schema), quoteIdent(ref.Name), strings.Join(cols, ",\n    "), quoteIdent(server))
	if tableOpts != "" {
		fmt.Fprintf(&sb, "\nOPTIONS (%s)", tableOpts)
	}
	sb.WriteString(";")
	return sb.String(), nil
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
