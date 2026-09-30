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
// data. crdb_internal and pg_extension have no PostgreSQL equivalent;
// postgres's own loaders already classify pg_catalog and information_schema.
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

func functionRequestedRef(refs []metadata.ObjectRef, kind, namespace, name string) metadata.ObjectRef {
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

// routineKinds maps the routine object kinds to their pg_proc.prokind code.
var routineKinds = map[string]string{"function": "f", "procedure": "p"}

// functionObjects mirrors postgres.FunctionObjects but drops the JOIN
// pg_language: CockroachDB's pg_catalog.pg_language compatibility table is
// always empty, so pg_proc.prolang never resolves against it and an inner
// join silently returns zero rows for every function. The language is
// instead recovered from the LANGUAGE clause pg_get_functiondef always
// emits. kind selects functions or procedures via routineKinds.
func functionObjects(ctx context.Context, db *sql.DB, kind string, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := functionPairFilter(refs, 1)
	q := `
SELECT n.nspname, p.proname,
       pg_get_function_arguments(p.oid),
       pg_get_function_result(p.oid),
       pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = $` + fmt.Sprint(len(args)+1) + ` AND (n.nspname, p.proname) IN (` + pairs + `)
ORDER BY n.nspname, p.proname, p.oid`
	rows, err := db.QueryContext(ctx, q, append(args, routineKinds[kind])...)
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
			Ref:         functionRequestedRef(refs, kind, ns, name),
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
func functionDefinition(ctx context.Context, db metadata.Querier, ref metadata.ObjectRef) (language, body string, err error) {
	rows, err := db.QueryContext(ctx, `
SELECT pg_get_functiondef(p.oid)
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE p.prokind = $3 AND n.nspname = $1 AND p.proname = $2
ORDER BY p.oid`, ref.Scope.Name("schema"), ref.Name, routineKinds[ref.Kind])
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
