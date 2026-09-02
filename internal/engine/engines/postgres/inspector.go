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

var _ metadata.SchemaInspector = (*postgresDriver)(nil)
var _ metadata.ScopeDiscoverer = (*postgresDriver)(nil)
var _ metadata.DefinitionInspector = (*postgresDriver)(nil)

func (d *postgresDriver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "postgres",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "materialized_view", Label: "Materialized View", PluralLabel: "Materialized Views", Order: 3, Relational: true, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "function", Label: "Function", PluralLabel: "Functions", Order: 4, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 5, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

func (d *postgresDriver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	var database string
	var currentSchema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&database, &currentSchema); err != nil {
		return nil, fmt.Errorf("postgres: directory database context: %w", err)
	}
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	defaultScope := root
	if currentSchema.Valid {
		defaultScope = root.Child(metadata.ScopeSegment{Kind: "schema", Name: currentSchema.String})
	}
	if d.defaultScope != "" {
		defaultScope = d.defaultScope
	}
	if opts.Root != "" {
		defaultScope = opts.Root
	}
	scope := func(namespace string) metadata.ScopePath {
		return root.Child(metadata.ScopeSegment{Kind: "schema", Name: namespace})
	}

	b := build.NewDirectory()
	b.AddScope(root)
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
		b.AddRef(scope(ns), kind, name)
	}); err != nil {
		return nil, fmt.Errorf("postgres: catalog tables: %w", err)
	}

	const mvQ = `
SELECT n.nspname, c.relname
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'm' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname`
	if err := d.queryRefs(ctx, mvQ, func(ns, name, _ string) { b.AddRef(scope(ns), "materialized_view", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog matviews: %w", err)
	}

	if err := d.attachRowCounts(ctx, b, scope); err != nil {
		return nil, fmt.Errorf("postgres: catalog row counts: %w", err)
	}

	const fnQ = `
SELECT n.nspname, p.proname
FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = 'f' AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, p.proname`
	if err := d.queryRefs(ctx, fnQ, func(ns, name, _ string) { b.AddRef(scope(ns), "function", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog functions: %w", err)
	}

	const seqQ = `
SELECT sequence_schema, sequence_name
FROM information_schema.sequences
WHERE sequence_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY sequence_schema, sequence_name`
	if err := d.queryRefs(ctx, seqQ, func(ns, name, _ string) { b.AddRef(scope(ns), "sequence", name) }); err != nil {
		return nil, fmt.Errorf("postgres: catalog sequences: %w", err)
	}

	return b.Build("", "postgres", defaultScope), nil
}

func (d *postgresDriver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	var currentDatabase string
	var currentSchema sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&currentDatabase, &currentSchema); err != nil {
		return nil, fmt.Errorf("postgres: discover current scope: %w", err)
	}
	currentRoot := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: currentDatabase})
	current := currentRoot
	if currentSchema.Valid {
		current = current.Child(metadata.ScopeSegment{Kind: "schema", Name: currentSchema.String})
	}
	result := &metadata.ScopeDiscovery{Current: current, Scopes: []metadata.ScopePath{}}
	parentDatabase := request.Parent.Name("database")
	if parentDatabase != "" {
		if parentDatabase != currentDatabase {
			return result, nil
		}
		rows, err := d.db.QueryContext(ctx, `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
  AND schema_name NOT LIKE 'pg_toast%'
  AND schema_name NOT LIKE 'pg_temp_%'
ORDER BY schema_name`)
		if err != nil {
			return nil, fmt.Errorf("postgres: discover schemas: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return nil, err
			}
			result.Scopes = append(result.Scopes, request.Parent.With("schema", name))
		}
		return result, rows.Err()
	}
	rows, err := d.db.QueryContext(ctx, `
SELECT datname
FROM pg_database
WHERE datallowconn AND NOT datistemplate AND has_database_privilege(datname, 'CONNECT')
ORDER BY datname`)
	if err != nil {
		return nil, fmt.Errorf("postgres: discover databases: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result.Scopes = append(result.Scopes, metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name}))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	schemaRows, err := d.db.QueryContext(ctx, `
SELECT schema_name
FROM information_schema.schemata
WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
  AND schema_name NOT LIKE 'pg_toast%'
  AND schema_name NOT LIKE 'pg_temp_%'
ORDER BY schema_name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: discover schemas: %w", err)
	}
	defer schemaRows.Close()
	for schemaRows.Next() {
		var name string
		if err := schemaRows.Scan(&name); err != nil {
			return nil, err
		}
		result.Scopes = append(result.Scopes, currentRoot.With("schema", name))
	}
	return result, schemaRows.Err()
}

// attachRowCounts sets the approximate row count (pg_class.reltuples) for
// every table and materialized view. reltuples is a planner statistic
// refreshed by ANALYZE/autovacuum, not a live COUNT(*), which is what keeps
// this query cheap regardless of table size.
func (d *postgresDriver) attachRowCounts(ctx context.Context, b *build.DirectoryBuilder, scope func(string) metadata.ScopePath) error {
	const q = `
SELECT n.nspname, c.relname, c.relkind, c.reltuples
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'm') AND n.nspname NOT IN ('pg_catalog', 'information_schema')`
	rows, err := d.db.QueryContext(ctx, q)
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
		b.SetRowCount(scope(ns), kind, name, int64(reltuples))
	}
	return rows.Err()
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

func (d *postgresDriver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs, mvRefs, fnRefs, seqRefs []metadata.ObjectRef
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

	var out []metadata.Object
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

func (d *postgresDriver) inspectRelational(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
	irows, err := d.db.QueryContext(ctx, idxQ, args...)
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
	if err := d.attachPostgresComments(ctx, out, pairs, args); err != nil {
		return nil, err
	}
	return out, nil
}

// InspectDefinition serves one object's canonical text definition on demand so
// bulk InspectObjects (and every schema snapshot) skips the per-object cost:
// table DDL is reconstructed from the catalog, views come from pg_get_viewdef,
// and functions from pg_get_functiondef. Unsupported kinds, or an object that no
// longer exists, yield a nil descriptor with a nil error.
//
// TODO: revisit Postgres table DDL generation. It currently covers columns
// (types, NOT NULL, defaults, identity), table constraints (PK/UNIQUE/FK/CHECK),
// and secondary indexes, but not: non-default identity/sequence options
// (START/INCREMENT), generated/stored columns, partitioning, inheritance,
// storage/WITH params, collations, EXCLUDE constraints, or comments. Output
// stays valid SQL, but is not a full pg_dump-fidelity reproduction.
func (d *postgresDriver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	switch ref.Kind {
	case "table":
		ddl, err := d.buildPostgresTableDDL(ctx, ref)
		if err != nil {
			return nil, err
		}
		return postgresSourceDescriptor("DDL", "sql", ddl), nil
	case "view":
		var def sql.NullString
		err := d.db.QueryRowContext(ctx,
			`SELECT pg_get_viewdef(format('%I.%I', $1::text, $2::text)::regclass, true)`,
			ref.Scope.Name("schema"), ref.Name).Scan(&def)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("postgres: view definition: %w", err)
		}
		return postgresSourceDescriptor("Definition", "sql", def.String), nil
	case "function":
		var lang, def sql.NullString
		err := d.db.QueryRowContext(ctx, `
SELECT l.lanname, pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
JOIN pg_language l ON l.oid = p.prolang
WHERE p.prokind = 'f' AND n.nspname = $1 AND p.proname = $2
ORDER BY p.oid
LIMIT 1`, ref.Scope.Name("schema"), ref.Name).Scan(&lang, &def)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("postgres: function definition: %w", err)
		}
		language := lang.String
		if language == "" {
			language = "sql"
		}
		return postgresSourceDescriptor("Definition", language, def.String), nil
	default:
		return nil, nil
	}
}

func postgresSourceDescriptor(title, language, body string) *metadata.Descriptor {
	if body == "" {
		return nil
	}
	return &metadata.Descriptor{
		Kind:   "source",
		Title:  title,
		Source: &metadata.Source{Language: language, Body: body},
	}
}

func (d *postgresDriver) buildPostgresTableDDL(ctx context.Context, ref metadata.ObjectRef) (string, error) {
	args := []any{ref.Scope.Name("schema"), ref.Name}
	relArg := `format('%I.%I', $1::text, $2::text)::regclass`

	var qualified string
	if err := d.db.QueryRowContext(ctx, `SELECT format('%I.%I', $1::text, $2::text)`, args...).Scan(&qualified); err != nil {
		return "", fmt.Errorf("postgres: ddl qualified name: %w", err)
	}

	// to_regclass yields NULL (not an error) for a relation that no longer
	// exists, so a stale ref resolves to an empty definition rather than the
	// hard error a bare ::regclass cast would raise.
	var exists sql.NullString
	if err := d.db.QueryRowContext(ctx, `SELECT to_regclass(format('%I.%I', $1::text, $2::text))`, args...).Scan(&exists); err != nil {
		return "", fmt.Errorf("postgres: ddl relation lookup: %w", err)
	}
	if !exists.Valid {
		return "", nil
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
func (d *postgresDriver) attachPostgresComments(ctx context.Context, objs []metadata.Object, pairs string, args []any) error {
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

func (d *postgresDriver) inspectMatviews(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
		b.AddColumn(refFor(ns, mv), metadata.Column{Name: col, DataType: dtype, Ordinal: attnum})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("postgres: matview columns rows: %w", err)
	}
	rows.Close()
	return b.Build(), nil
}

func (d *postgresDriver) inspectFunctions(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
ORDER BY n.nspname, p.proname`
	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: function detail: %w", err)
	}
	defer rows.Close()
	var out []metadata.Object
	for rows.Next() {
		var ns, name, fnArgs, lang string
		var ret sql.NullString
		if err := rows.Scan(&ns, &name, &fnArgs, &ret, &lang); err != nil {
			return nil, fmt.Errorf("postgres: function detail scan: %w", err)
		}
		fields := []metadata.Field{
			{Name: "Arguments", Value: fnArgs},
			{Name: "Language", Value: lang},
		}
		if ret.Valid {
			fields = append(fields, metadata.Field{Name: "Returns", Value: ret.String})
		}
		out = append(out, metadata.Object{
			Ref: postgresRequestedRef(refs, ns, name, "function"),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Signature", Fields: fields},
			},
		})
	}
	return out, rows.Err()
}

func (d *postgresDriver) inspectSequences(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
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
