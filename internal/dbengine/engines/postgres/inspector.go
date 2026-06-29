package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/dbengine/schema"
	build "github.com/sqlwarden/internal/dbengine/schema/build"
)

var _ schema.SchemaInspector = (*postgresDriver)(nil)

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
		objs, err := d.inspectRelational(ctx, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(mvRefs) > 0 {
		objs, err := d.inspectMatviews(ctx, mvRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(fnRefs) > 0 {
		objs, err := d.inspectFunctions(ctx, fnRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(seqRefs) > 0 {
		objs, err := d.inspectSequences(ctx, seqRefs)
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

func (d *postgresDriver) inspectRelational(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
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

	out := b.Build()
	if err := d.attachPostgresComments(ctx, out, pairs, args); err != nil {
		return nil, err
	}
	if err := d.attachPostgresViewDefinitions(ctx, out); err != nil {
		return nil, err
	}
	if err := d.attachPostgresTableDDL(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachPostgresTableDDL reconstructs a CREATE TABLE statement from the catalog
// (Postgres has no SHOW CREATE TABLE) and attaches it as a "DDL" source
// descriptor.
//
// TODO: revisit Postgres table DDL generation. It currently covers columns
// (types, NOT NULL, defaults, identity), table constraints (PK/UNIQUE/FK/CHECK),
// and secondary indexes, but not: non-default identity/sequence options
// (START/INCREMENT), generated/stored columns, partitioning, inheritance,
// storage/WITH params, collations, EXCLUDE constraints, or comments. Output
// stays valid SQL, but is not a full pg_dump-fidelity reproduction.
func (d *postgresDriver) attachPostgresTableDDL(ctx context.Context, objs []schema.Object) error {
	for i := range objs {
		if objs[i].Ref.Kind != "table" {
			continue
		}
		ddl, err := d.buildPostgresTableDDL(ctx, objs[i].Ref)
		if err != nil {
			return err
		}
		if ddl != "" {
			objs[i].Descriptors = append(objs[i].Descriptors, schema.Descriptor{
				Kind:   "source",
				Title:  "DDL",
				Source: &schema.Source{Language: "sql", Body: ddl},
			})
		}
	}
	return nil
}

func (d *postgresDriver) buildPostgresTableDDL(ctx context.Context, ref schema.ObjectRef) (string, error) {
	args := []any{ref.Namespace, ref.Name}
	relArg := `format('%I.%I', $1::text, $2::text)::regclass`

	var qualified string
	if err := d.db.QueryRowContext(ctx, `SELECT format('%I.%I', $1::text, $2::text)`, args...).Scan(&qualified); err != nil {
		return "", fmt.Errorf("postgres: ddl qualified name: %w", err)
	}

	var lines []string

	colRows, err := d.db.QueryContext(ctx, `
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

	conRows, err := d.db.QueryContext(ctx, `
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

	idxRows, err := d.db.QueryContext(ctx, `
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

// attachPostgresComments populates table and column "comment" attributes from
// obj_description / col_description.
func (d *postgresDriver) attachPostgresComments(ctx context.Context, objs []schema.Object, pairs string, args []any) error {
	tableComments := map[string]string{}
	trows, err := d.db.QueryContext(ctx, `
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
	crows, err := d.db.QueryContext(ctx, `
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
		ns, name := objs[i].Ref.Namespace, objs[i].Ref.Name
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

// attachPostgresViewDefinitions appends each view's definition as a "source"
// descriptor via pg_get_viewdef.
func (d *postgresDriver) attachPostgresViewDefinitions(ctx context.Context, objs []schema.Object) error {
	for i := range objs {
		if objs[i].Ref.Kind != "view" {
			continue
		}
		var def sql.NullString
		err := d.db.QueryRowContext(ctx,
			`SELECT pg_get_viewdef(format('%I.%I', $1::text, $2::text)::regclass, true)`,
			objs[i].Ref.Namespace, objs[i].Ref.Name).Scan(&def)
		if err != nil {
			return fmt.Errorf("postgres: view definition: %w", err)
		}
		if def.Valid && def.String != "" {
			objs[i].Descriptors = append(objs[i].Descriptors, schema.Descriptor{
				Kind:   "source",
				Title:  "Definition",
				Source: &schema.Source{Language: "sql", Body: def.String},
			})
		}
	}
	return nil
}

func setObjectAttr(o *schema.Object, key, value string) {
	if value == "" {
		return
	}
	if o.Attributes == nil {
		o.Attributes = map[string]any{}
	}
	o.Attributes[key] = value
}

func setColumnAttr(c *schema.Column, key, value string) {
	if value == "" {
		return
	}
	if c.Attributes == nil {
		c.Attributes = map[string]any{}
	}
	c.Attributes[key] = value
}

func (d *postgresDriver) inspectMatviews(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
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

func (d *postgresDriver) inspectFunctions(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
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

func (d *postgresDriver) inspectSequences(ctx context.Context, refs []schema.ObjectRef) ([]schema.Object, error) {
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
