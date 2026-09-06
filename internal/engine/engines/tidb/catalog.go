package tidb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// CatalogTables enumerates every table and view in database, invoking add
// once per object with its schema, name, and resolved kind ("table" or
// "view"). Unlike mysql.CatalogTables, it excludes TiDB's native SEQUENCE
// objects — information_schema.tables.table_type reports them as "SEQUENCE",
// a value MySQL's table_type never produces — so callers must combine this
// with CatalogSequences rather than mysql.CatalogTables to avoid misreporting
// sequences as plain tables.
func CatalogTables(ctx context.Context, db *sql.DB, database string, add func(schema, name, kind string)) error {
	const q = `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema = ? AND table_type <> 'SEQUENCE'
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

// CatalogSequences enumerates every native TiDB sequence in database.
func CatalogSequences(ctx context.Context, db *sql.DB, database string, add func(schema, name string)) error {
	const q = `
SELECT table_schema, table_name
FROM information_schema.tables
WHERE table_schema = ? AND table_type = 'SEQUENCE'
ORDER BY table_schema, table_name`
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

func tidbPairFilter(refs []metadata.ObjectRef) (string, []any) {
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

// SequenceObjects fetches storage-engine detail for sequences named in refs.
// TiDB's own SEQUENCE definition (start/increment/min/max/cache/cycle) is
// served lazily through InspectDefinition's "SHOW CREATE SEQUENCE", matching
// how table DDL and view bodies are handled elsewhere in this engine family —
// this only surfaces the cheap information_schema.tables detail.
func SequenceObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := tidbPairFilter(refs)
	q := `
SELECT table_schema, table_name, engine
FROM information_schema.tables
WHERE (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("tidb: sequence detail: %w", err)
	}
	defer rows.Close()
	var out []metadata.Object
	for rows.Next() {
		var ns, name string
		var eng sql.NullString
		if err := rows.Scan(&ns, &name, &eng); err != nil {
			return nil, fmt.Errorf("tidb: sequence detail scan: %w", err)
		}
		out = append(out, metadata.Object{
			Ref: tidbRequestedRef(refs, ns, name, "sequence"),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Sequence", Fields: []metadata.Field{{Name: "Engine", Value: eng.String}}},
			},
		})
	}
	return out, rows.Err()
}

func tidbRequestedRef(refs []metadata.ObjectRef, database, name, kind string) metadata.ObjectRef {
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
