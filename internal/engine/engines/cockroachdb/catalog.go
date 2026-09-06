package cockroachdb

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// systemSchemas are CockroachDB's built-in virtual schemas that are not user
// data and must be excluded from catalog listings and scope discovery.
// Callers filter postgres's shared catalog.go functions in Go rather than
// re-deriving their SQL. crdb_internal and pg_extension have no PostgreSQL
// equivalent; pg_catalog and information_schema are already excluded inside
// the shared functions themselves.
var systemSchemas = map[string]bool{
	"crdb_internal": true,
	"pg_extension":  true,
}

// languageFromDefinition extracts the LANGUAGE clause CockroachDB always
// emits in pg_get_functiondef's output, since pg_proc.prolang does not
// resolve against pg_catalog.pg_language (see functionObjects).
var languageClause = regexp.MustCompile(`(?i)LANGUAGE\s+(\w+)`)

func languageFromDefinition(def string) string {
	m := languageClause.FindStringSubmatch(def)
	if m == nil {
		return ""
	}
	return strings.ToLower(m[1])
}

// functionPairFilter builds a "($n,$n+1),($n+2,$n+3),…" tuple list plus the
// flattened (namespace, name) args, for a "(schema, name) IN (...)"
// predicate — a local copy of postgres's unexported pairFilter.
func functionPairFilter(refs []metadata.ObjectRef, start int) (string, []any) {
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

func functionRequestedRef(refs []metadata.ObjectRef, namespace, name string) metadata.ObjectRef {
	for _, ref := range refs {
		if ref.Scope.Name("schema") == namespace && ref.Name == name {
			return ref
		}
	}
	var scope metadata.ScopePath
	if len(refs) > 0 {
		scope = refs[0].Scope.With("schema", namespace)
	}
	return metadata.ObjectRef{Scope: scope, Kind: "function", Name: name}
}

// functionObjects mirrors postgres.FunctionObjects but drops the JOIN
// pg_language: CockroachDB's pg_catalog.pg_language compatibility table is
// always empty, so pg_proc.prolang never resolves against it and an inner
// join silently returns zero rows for every function. The language is
// instead recovered from the LANGUAGE clause pg_get_functiondef always
// emits.
func functionObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := functionPairFilter(refs, 1)
	q := `
SELECT n.nspname, p.proname,
       pg_get_function_arguments(p.oid),
       pg_get_function_result(p.oid),
       pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = 'f' AND (n.nspname, p.proname) IN (` + pairs + `)
ORDER BY n.nspname, p.proname, p.oid`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("cockroachdb: function detail: %w", err)
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
			title := "Signature"
			if len(overloads) > 1 {
				title = fmt.Sprintf("Signature (overload %d of %d)", i+1, len(overloads))
			}
			descriptors = append(descriptors, metadata.Descriptor{Kind: "fields", Title: title, Fields: fields})
		}
		out = append(out, metadata.Object{
			Ref:         functionRequestedRef(refs, ns, name),
			Descriptors: descriptors,
		})
		overloads = nil
	}
	for rows.Next() {
		var rowNS, rowName, fnArgs, def string
		var ret sql.NullString
		if err := rows.Scan(&rowNS, &rowName, &fnArgs, &ret, &def); err != nil {
			return nil, fmt.Errorf("cockroachdb: function detail scan: %w", err)
		}
		if rowNS != ns || rowName != name {
			flush()
			ns, name = rowNS, rowName
		}
		fields := []metadata.Field{
			{Name: "Arguments", Value: fnArgs},
			{Name: "Language", Value: languageFromDefinition(def)},
		}
		if ret.Valid {
			fields = append(fields, metadata.Field{Name: "Returns", Value: ret.String})
		}
		overloads = append(overloads, fields)
	}
	flush()
	return out, rows.Err()
}

// functionDefinition mirrors postgres.FunctionDefinition but, like
// functionObjects, recovers the language from pg_get_functiondef's own
// LANGUAGE clause instead of joining pg_language.
func functionDefinition(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (language, body string, err error) {
	rows, err := db.QueryContext(ctx, `
SELECT pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = 'f' AND n.nspname = $1 AND p.proname = $2
ORDER BY p.oid`, ref.Scope.Name("schema"), ref.Name)
	if err != nil {
		return "", "", fmt.Errorf("cockroachdb: function definition: %w", err)
	}
	defer rows.Close()

	var defs []string
	for rows.Next() {
		var def sql.NullString
		if err := rows.Scan(&def); err != nil {
			return "", "", fmt.Errorf("cockroachdb: function definition scan: %w", err)
		}
		defs = append(defs, def.String)
	}
	if err := rows.Err(); err != nil {
		return "", "", fmt.Errorf("cockroachdb: function definition: %w", err)
	}
	if len(defs) == 0 {
		return "", "", nil
	}
	language = languageFromDefinition(defs[0])
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

// attachRowCounts reports the approximate row count (pg_class.reltuples) for
// every table. CockroachDB's pg_class compatibility view returns NULL for
// reltuples until a table has been scanned by its stats collector, whereas
// PostgreSQL always reports a real (possibly zero) value — so unlike
// postgres.AttachRowCounts, this scans reltuples as a nullable column and
// simply omits the row count for a table that hasn't been analyzed yet.
// System-schema exclusion is left to the caller (inspector.go), matching how
// postgres.AttachRowCounts itself only excludes pg_catalog/information_schema.
func attachRowCounts(ctx context.Context, db *sql.DB, set func(schema, name string, count int64)) error {
	const q = `
SELECT n.nspname, c.relname, c.reltuples
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind = 'r' AND n.nspname NOT IN ('pg_catalog', 'information_schema')`
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ns, name string
		var reltuples sql.NullFloat64
		if err := rows.Scan(&ns, &name, &reltuples); err != nil {
			return err
		}
		if !reltuples.Valid || reltuples.Float64 < 0 {
			continue
		}
		set(ns, name, int64(reltuples.Float64))
	}
	return rows.Err()
}
