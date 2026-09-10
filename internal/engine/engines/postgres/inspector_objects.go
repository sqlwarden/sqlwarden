package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

// ProcedureObjects fetches argument/language detail for procedures named in
// refs, mirroring FunctionObjects but against prokind = 'p'. Procedures are
// overloadable, so several pg_proc rows can map to one (schema, name) ref;
// each is emitted as its own "overload" descriptor under a single object.
func ProcedureObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT n.nspname, p.proname, pg_get_function_arguments(p.oid), l.lanname
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
JOIN pg_language l ON l.oid = p.prolang
WHERE p.prokind = 'p' AND (n.nspname, p.proname) IN (` + pairs + `)
ORDER BY n.nspname, p.proname, p.oid`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: procedure detail: %w", err)
	}
	defer rows.Close()

	var out []metadata.Object
	var ns, name string
	var overloads [][]metadata.Field
	flush := func() {
		if len(overloads) == 0 {
			return
		}
		descriptors := make([]metadata.Descriptor, 0, len(overloads))
		for i, fields := range overloads {
			title := "Procedure"
			if len(overloads) > 1 {
				title = fmt.Sprintf("Procedure (overload %d of %d)", i+1, len(overloads))
			}
			descriptors = append(descriptors, metadata.Descriptor{Kind: "fields", Title: title, Fields: fields})
		}
		out = append(out, metadata.Object{
			Ref:         postgresRequestedRef(refs, ns, name, "procedure"),
			Descriptors: descriptors,
		})
		overloads = nil
	}
	for rows.Next() {
		var rowNS, rowName, procArgs, lang string
		if err := rows.Scan(&rowNS, &rowName, &procArgs, &lang); err != nil {
			return nil, fmt.Errorf("postgres: procedure detail scan: %w", err)
		}
		if rowNS != ns || rowName != name {
			flush()
			ns, name = rowNS, rowName
		}
		overloads = append(overloads, []metadata.Field{
			{Name: "Arguments", Value: procArgs},
			{Name: "Language", Value: lang},
		})
	}
	flush()
	return out, rows.Err()
}

// TriggerObjects fetches timing/event/table detail for triggers named in
// refs, via pg_trigger's action metadata (tgtype is a bitmask decoded
// through pg_get_triggerdef for a human-readable statement instead).
//
// A Postgres trigger name is unique per table, not per schema, so the same
// name may sit on several tables in one schema. The directory keys objects by
// (scope, kind, name), which cannot hold more than one such trigger, so every
// row for a given name is folded into a single object that lists all bearing
// tables and their definitions.
func TriggerObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT n.nspname, t.tgname, tbl.relname, pg_get_triggerdef(t.oid)
FROM pg_trigger t
JOIN pg_class tbl ON tbl.oid = t.tgrelid
JOIN pg_namespace n ON n.oid = tbl.relnamespace
WHERE NOT t.tgisinternal AND (n.nspname, t.tgname) IN (` + pairs + `)
ORDER BY n.nspname, t.tgname, tbl.relname`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: trigger detail: %w", err)
	}
	defer rows.Close()

	type triggerDetail struct {
		ref    metadata.ObjectRef
		tables []string
		defs   []string
	}
	order := make([]string, 0, len(refs))
	byName := map[string]*triggerDetail{}
	for rows.Next() {
		var ns, name, table, def string
		if err := rows.Scan(&ns, &name, &table, &def); err != nil {
			return nil, fmt.Errorf("postgres: trigger detail scan: %w", err)
		}
		key := ns + "." + name
		detail, ok := byName[key]
		if !ok {
			detail = &triggerDetail{ref: postgresRequestedRef(refs, ns, name, "trigger")}
			byName[key] = detail
			order = append(order, key)
		}
		detail.tables = append(detail.tables, table)
		detail.defs = append(detail.defs, def)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]metadata.Object, 0, len(order))
	for _, key := range order {
		detail := byName[key]
		out = append(out, metadata.Object{
			Ref: detail.ref,
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Trigger", Fields: []metadata.Field{{Name: "Table", Value: strings.Join(detail.tables, ", ")}}},
				{Kind: "source", Title: "Definition", Source: &metadata.Source{Language: "sql", Body: strings.Join(detail.defs, "\n\n")}},
			},
		})
	}
	return out, nil
}

// TypeObjects fetches category and member detail for composite/enum/range
// types named in refs.
func TypeObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT n.nspname, t.typname, t.typtype
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE (n.nspname, t.typname) IN (` + pairs + `)`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: type detail: %w", err)
	}
	defer rows.Close()
	category := map[string]string{"c": "Composite", "e": "Enum", "r": "Range"}
	var out []metadata.Object
	for rows.Next() {
		var ns, name, typtype string
		if err := rows.Scan(&ns, &name, &typtype); err != nil {
			return nil, fmt.Errorf("postgres: type detail scan: %w", err)
		}
		out = append(out, metadata.Object{
			Ref: postgresRequestedRef(refs, ns, name, "type"),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Type", Fields: []metadata.Field{{Name: "Category", Value: category[typtype]}}},
			},
		})
	}
	return out, rows.Err()
}

// DomainObjects fetches base type and constraint detail for domains named in
// refs.
func DomainObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := pairFilter(refs, 1)
	q := `
SELECT n.nspname, t.typname, format_type(t.typbasetype, t.typtypmod),
       COALESCE((SELECT string_agg(pg_get_constraintdef(con.oid), ' AND ') FROM pg_constraint con WHERE con.contypid = t.oid), '')
FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace
WHERE t.typtype = 'd' AND (n.nspname, t.typname) IN (` + pairs + `)`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: domain detail: %w", err)
	}
	defer rows.Close()
	var out []metadata.Object
	for rows.Next() {
		var ns, name, baseType, constraint string
		if err := rows.Scan(&ns, &name, &baseType, &constraint); err != nil {
			return nil, fmt.Errorf("postgres: domain detail scan: %w", err)
		}
		fields := []metadata.Field{{Name: "Base type", Value: baseType}}
		if constraint != "" {
			fields = append(fields, metadata.Field{Name: "Constraint", Value: constraint})
		}
		out = append(out, metadata.Object{
			Ref:         postgresRequestedRef(refs, ns, name, "domain"),
			Descriptors: []metadata.Descriptor{{Kind: "fields", Title: "Domain", Fields: fields}},
		})
	}
	return out, rows.Err()
}

// ForeignTableObjects fetches column detail for foreign tables named in
// refs, reusing information_schema.columns like RelationalObjects since
// foreign tables expose the same column catalog shape.
func ForeignTableObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	b := build.NewRelational()
	for _, r := range refs {
		b.Ensure(r)
	}
	refFor := func(ns, name string) metadata.ObjectRef { return postgresRequestedRef(refs, ns, name, "foreign_table") }
	pairs, args := pairFilter(refs, 1)
	colQ := `
SELECT table_schema, table_name, column_name, udt_name, is_nullable, ordinal_position
FROM information_schema.columns
WHERE (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, ordinal_position`
	rows, err := db.QueryContext(ctx, colQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: foreign table columns: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ns, tbl, col, dtype, nullable string
		var ord int
		if err := rows.Scan(&ns, &tbl, &col, &dtype, &nullable, &ord); err != nil {
			return nil, fmt.Errorf("postgres: foreign table columns scan: %w", err)
		}
		b.AddColumn(refFor(ns, tbl), metadata.Column{Name: col, DataType: dtype, Nullable: nullable == "YES", Ordinal: ord})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: foreign table columns rows: %w", err)
	}

	serverQ := `
SELECT foreign_table_schema, foreign_table_name, foreign_server_name
FROM information_schema.foreign_tables
WHERE (foreign_table_schema, foreign_table_name) IN (` + pairs + `)`
	srows, err := db.QueryContext(ctx, serverQ, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: foreign table server: %w", err)
	}
	defer srows.Close()
	servers := map[string]string{}
	for srows.Next() {
		var ns, tbl, server string
		if err := srows.Scan(&ns, &tbl, &server); err != nil {
			return nil, fmt.Errorf("postgres: foreign table server scan: %w", err)
		}
		servers[ns+"\x00"+tbl] = server
	}
	if err := srows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: foreign table server rows: %w", err)
	}

	out := b.Build()
	for i := range out {
		if server, ok := servers[out[i].Ref.Scope.Name("schema")+"\x00"+out[i].Ref.Name]; ok {
			setObjectAttr(&out[i], "server", server)
		}
	}
	return out, nil
}

// attachPostgresPartitions adds a "Partitions" rows descriptor to every
// partitioned table object (relkind = 'p') in objs, listing each partition's
// name, bound expression, and estimated row count. Non-partitioned tables
// and tables with no partitions attached are left unchanged.
func attachPostgresPartitions(ctx context.Context, db *sql.DB, objs []metadata.Object) error {
	var tableRefs []metadata.ObjectRef
	for _, obj := range objs {
		if obj.Ref.Kind == "table" {
			tableRefs = append(tableRefs, obj.Ref)
		}
	}
	if len(tableRefs) == 0 {
		return nil
	}
	pairs, args := pairFilter(tableRefs, 1)
	q := `
SELECT pn.nspname, parent.relname, child.relname, pg_get_expr(child.relpartbound, child.oid), child.reltuples
FROM pg_inherits inh
JOIN pg_class parent ON parent.oid = inh.inhparent
JOIN pg_namespace pn ON pn.oid = parent.relnamespace
JOIN pg_class child ON child.oid = inh.inhrelid
WHERE parent.relkind = 'p' AND (pn.nspname, parent.relname) IN (` + pairs + `)
ORDER BY pn.nspname, parent.relname, child.relname`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("postgres: partitions: %w", err)
	}
	defer rows.Close()
	type partitionRow struct{ name, bound, count string }
	partitions := map[string][]partitionRow{}
	for rows.Next() {
		var ns, parent, child, bound string
		var reltuples float64
		if err := rows.Scan(&ns, &parent, &child, &bound, &reltuples); err != nil {
			return fmt.Errorf("postgres: partitions scan: %w", err)
		}
		count := ""
		if reltuples >= 0 {
			count = strconv.FormatInt(int64(reltuples), 10)
		}
		key := ns + "\x00" + parent
		partitions[key] = append(partitions[key], partitionRow{name: child, bound: bound, count: count})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("postgres: partitions rows: %w", err)
	}
	for i := range objs {
		key := objs[i].Ref.Scope.Name("schema") + "\x00" + objs[i].Ref.Name
		list, ok := partitions[key]
		if !ok {
			continue
		}
		partitionRows := make([][]string, len(list))
		for j, p := range list {
			partitionRows[j] = []string{p.name, p.bound, p.count}
		}
		objs[i].Descriptors = append(objs[i].Descriptors, metadata.Descriptor{
			Kind:  "rows",
			Title: "Partitions",
			Rows:  &metadata.RowSet{Columns: []string{"Partition", "Bound", "Rows"}, Rows: partitionRows},
		})
	}
	return nil
}
