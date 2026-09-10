package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// CatalogEvents enumerates every scheduled event in database.
func CatalogEvents(ctx context.Context, db *sql.DB, database string, add func(schema, name string)) error {
	const q = `
SELECT event_schema, event_name
FROM information_schema.events
WHERE event_schema = ?
ORDER BY event_name`
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

// EventObjects fetches schedule/status detail for events named in refs.
func EventObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT event_schema, event_name, event_type, interval_value, interval_field, status
FROM information_schema.events
WHERE (event_schema, event_name) IN (` + pairs + `)`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: event detail: %w", err)
	}
	defer rows.Close()
	var out []metadata.Object
	for rows.Next() {
		var ns, name, eventType, status string
		var intervalValue, intervalField sql.NullString
		if err := rows.Scan(&ns, &name, &eventType, &intervalValue, &intervalField, &status); err != nil {
			return nil, fmt.Errorf("mysql: event detail scan: %w", err)
		}
		fields := []metadata.Field{
			{Name: "Type", Value: eventType},
			{Name: "Status", Value: status},
		}
		if intervalValue.Valid && intervalField.Valid {
			fields = append(fields, metadata.Field{Name: "Interval", Value: intervalValue.String + " " + intervalField.String})
		}
		out = append(out, metadata.Object{
			Ref:         mysqlRequestedRef(refs, ns, name, "event"),
			Descriptors: []metadata.Descriptor{{Kind: "fields", Title: "Event", Fields: fields}},
		})
	}
	return out, rows.Err()
}

// CatalogIndexes enumerates every secondary index (excluding PRIMARY) in
// database, one entry per (table, index) pair — matching Oracle's
// promotion of indexes to a first-class browsable kind.
//
// Unlike Postgres/Oracle where (schema, name) uniquely identifies an index,
// MySQL index names are scoped per-table, so two different tables in the
// same schema can share an index name. CatalogIndexes emits duplicate
// (schema, name) refs that collapse to one tree entry — an accepted
// limitation matching how CatalogFunctions collapses Postgres overloads
// under one name. IndexObjects surfaces every table's instance below rather
// than picking one arbitrarily.
func CatalogIndexes(ctx context.Context, db *sql.DB, database string, add func(schema, name string)) error {
	const q = `
SELECT DISTINCT table_schema, index_name
FROM information_schema.statistics
WHERE table_schema = ? AND index_name <> 'PRIMARY'
ORDER BY index_name`
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

// CatalogConstraints enumerates every table constraint (PK/UNIQUE/FK/CHECK)
// in database. Like CatalogIndexes, constraint names are scoped per-table in
// MySQL, so (schema, name) can span multiple tables; see CatalogIndexes. The
// DISTINCT is what collapses those to one entry: information_schema.
// table_constraints has a row per (constraint, table), so without it every
// table's PRIMARY constraint would be emitted as its own duplicate ref.
func CatalogConstraints(ctx context.Context, db *sql.DB, database string, add func(schema, name string)) error {
	const q = `
SELECT DISTINCT table_schema, constraint_name
FROM information_schema.table_constraints
WHERE table_schema = ?
ORDER BY constraint_name`
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

// IndexObjects fetches column/uniqueness/table detail for indexes named in
// refs. A name may span multiple tables (see CatalogIndexes); every table's
// instance is rendered as its own descriptor.
func IndexObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT table_schema, index_name, table_name, non_unique, column_name, seq_in_index
FROM information_schema.statistics
WHERE (table_schema, index_name) IN (` + pairs + `)
ORDER BY table_schema, index_name, table_name, seq_in_index`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: index detail: %w", err)
	}
	defer rows.Close()
	type instance struct {
		table   string
		unique  bool
		columns []string
	}
	byKey := map[string][]*instance{}
	for rows.Next() {
		var ns, name, table, col string
		var nonUnique, seq int
		if err := rows.Scan(&ns, &name, &table, &nonUnique, &col, &seq); err != nil {
			return nil, fmt.Errorf("mysql: index detail scan: %w", err)
		}
		key := ns + "\x00" + name
		var inst *instance
		for _, candidate := range byKey[key] {
			if candidate.table == table {
				inst = candidate
				break
			}
		}
		if inst == nil {
			inst = &instance{table: table, unique: nonUnique == 0}
			byKey[key] = append(byKey[key], inst)
		}
		inst.columns = append(inst.columns, col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: index detail rows: %w", err)
	}
	var out []metadata.Object
	for _, ref := range refs {
		key := ref.Scope.Name("database") + "\x00" + ref.Name
		var descriptors []metadata.Descriptor
		for _, inst := range byKey[key] {
			descriptors = append(descriptors, metadata.Descriptor{
				Kind: "fields", Title: "Index on " + inst.table,
				Fields: []metadata.Field{
					{Name: "Table", Value: inst.table},
					{Name: "Unique", Value: fmt.Sprintf("%v", inst.unique)},
					{Name: "Columns", Value: strings.Join(inst.columns, ", ")},
				},
			})
		}
		out = append(out, metadata.Object{Ref: ref, Descriptors: descriptors})
	}
	return out, nil
}

// ConstraintObjects fetches type/table detail for constraints named in refs,
// same multi-table-instance shape as IndexObjects. FOREIGN KEY constraints
// additionally surface their referenced table/column via a join against
// key_column_usage, mirroring Oracle's constraint kind rendering the FK
// target (all_constraints.r_owner/r_constraint_name). Non-FK constraint
// types (PRIMARY KEY, UNIQUE, CHECK) have no referenced target, so those
// descriptors keep the plain Table/Type shape.
func ConstraintObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := mysqlPairFilter(refs)
	q := `
SELECT tc.table_schema, tc.constraint_name, tc.table_name, tc.constraint_type,
       kcu.referenced_table_schema, kcu.referenced_table_name, kcu.referenced_column_name
FROM information_schema.table_constraints tc
LEFT JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_schema = tc.constraint_schema
  AND kcu.constraint_name = tc.constraint_name
  AND kcu.table_schema = tc.table_schema
  AND kcu.table_name = tc.table_name
  AND kcu.referenced_table_name IS NOT NULL
WHERE (tc.table_schema, tc.constraint_name) IN (` + pairs + `)
ORDER BY tc.table_schema, tc.constraint_name, tc.table_name, kcu.ordinal_position`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql: constraint detail: %w", err)
	}
	defer rows.Close()
	type row struct {
		table, kind string
		refTable    string
		refColumns  []string
	}
	byKey := map[string][]*row{}
	for rows.Next() {
		var ns, name, table, kind string
		var refSchema, refTable, refColumn sql.NullString
		if err := rows.Scan(&ns, &name, &table, &kind, &refSchema, &refTable, &refColumn); err != nil {
			return nil, fmt.Errorf("mysql: constraint detail scan: %w", err)
		}
		key := ns + "\x00" + name
		var r *row
		for _, candidate := range byKey[key] {
			if candidate.table == table {
				r = candidate
				break
			}
		}
		if r == nil {
			r = &row{table: table, kind: kind}
			byKey[key] = append(byKey[key], r)
		}
		if kind == "FOREIGN KEY" && refTable.Valid && refColumn.Valid {
			r.refTable = refTable.String
			r.refColumns = append(r.refColumns, refColumn.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: constraint detail rows: %w", err)
	}
	var out []metadata.Object
	for _, ref := range refs {
		key := ref.Scope.Name("database") + "\x00" + ref.Name
		var descriptors []metadata.Descriptor
		for _, r := range byKey[key] {
			fields := []metadata.Field{{Name: "Table", Value: r.table}, {Name: "Type", Value: r.kind}}
			if r.kind == "FOREIGN KEY" && r.refTable != "" {
				fields = append(fields,
					metadata.Field{Name: "Referenced table", Value: r.refTable},
					metadata.Field{Name: "Referenced column", Value: strings.Join(r.refColumns, ", ")},
				)
			}
			descriptors = append(descriptors, metadata.Descriptor{
				Kind: "fields", Title: "Constraint on " + r.table,
				Fields: fields,
			})
		}
		out = append(out, metadata.Object{Ref: ref, Descriptors: descriptors})
	}
	return out, nil
}

// attachMySQLPartitions adds a "Partitions" rows descriptor (name, method,
// expression, row count) to every partitioned table object in objs, mirroring
// Postgres's attachPostgresPartitions. Non-partitioned tables (a single NULL
// PARTITION_NAME row per information_schema.PARTITIONS) are left unchanged.
func attachMySQLPartitions(ctx context.Context, db *sql.DB, objs []metadata.Object) error {
	var tableRefs []metadata.ObjectRef
	for _, obj := range objs {
		if obj.Ref.Kind == "table" {
			tableRefs = append(tableRefs, obj.Ref)
		}
	}
	if len(tableRefs) == 0 {
		return nil
	}
	pairs, args := mysqlPairFilter(tableRefs)
	q := `
SELECT table_schema, table_name, partition_name, partition_method, partition_expression, table_rows
FROM information_schema.partitions
WHERE partition_name IS NOT NULL AND (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name, partition_ordinal_position`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("mysql: partitions: %w", err)
	}
	defer rows.Close()
	type partitionRow struct{ name, method, expression, count string }
	partitions := map[string][]partitionRow{}
	for rows.Next() {
		var ns, tbl, name string
		var method, expression sql.NullString
		var rowCount sql.NullInt64
		if err := rows.Scan(&ns, &tbl, &name, &method, &expression, &rowCount); err != nil {
			return fmt.Errorf("mysql: partitions scan: %w", err)
		}
		count := ""
		if rowCount.Valid {
			count = strconv.FormatInt(rowCount.Int64, 10)
		}
		key := ns + "\x00" + tbl
		partitions[key] = append(partitions[key], partitionRow{name: name, method: method.String, expression: expression.String, count: count})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mysql: partitions rows: %w", err)
	}
	for i := range objs {
		key := objs[i].Ref.Scope.Name("database") + "\x00" + objs[i].Ref.Name
		list, ok := partitions[key]
		if !ok {
			continue
		}
		setObjectAttr(&objs[i], "partition_method", list[0].method)
		partitionRows := make([][]string, len(list))
		for j, p := range list {
			partitionRows[j] = []string{p.name, p.method, p.expression, p.count}
		}
		objs[i].Descriptors = append(objs[i].Descriptors, metadata.Descriptor{
			Kind:  "rows",
			Title: "Partitions",
			Rows:  &metadata.RowSet{Columns: []string{"Partition", "Method", "Expression", "Rows"}, Rows: partitionRows},
		})
	}
	return nil
}
