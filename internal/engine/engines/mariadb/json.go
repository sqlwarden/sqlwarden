package mariadb

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// jsonCheckPattern matches the auto-generated CHECK MariaDB attaches to a
// column declared as JSON, e.g. ``json_valid(`payload`)``. MariaDB
// implements JSON as an alias for LONGTEXT: information_schema.columns
// reports the storage type, not the declared one, so this check constraint is
// the only place the original JSON declaration survives.
var jsonCheckPattern = regexp.MustCompile("(?i)json_valid\\(`?([A-Za-z0-9_$]+)`?\\)")

// jsonColumns returns the set of (schema, table, column) triples backed by a
// MariaDB auto-JSON check constraint, keyed by "schema\x00table\x00column".
func jsonColumns(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) (map[string]bool, error) {
	pairs, args := mariadbPairFilter(refs)
	q := `
SELECT tc.table_schema, tc.table_name, cc.check_clause
FROM information_schema.table_constraints tc
JOIN information_schema.check_constraints cc
  ON cc.constraint_schema = tc.constraint_schema AND cc.constraint_name = tc.constraint_name
WHERE tc.constraint_type = 'CHECK'
  AND (tc.table_schema, tc.table_name) IN (` + pairs + `)`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mariadb: json check constraints: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var ns, tbl, clause string
		if err := rows.Scan(&ns, &tbl, &clause); err != nil {
			return nil, fmt.Errorf("mariadb: json check constraints scan: %w", err)
		}
		match := jsonCheckPattern.FindStringSubmatch(clause)
		if match == nil {
			continue
		}
		out[ns+"\x00"+tbl+"\x00"+match[1]] = true
	}
	return out, rows.Err()
}

// patchJSONColumns rewrites the reported data type of every JSON-backed
// column in objs from "longtext" to "json", so the schema browser shows what
// the user declared rather than MariaDB's underlying storage
// representation. refs must be the same table/view refs objs was built from.
func patchJSONColumns(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef, objs []metadata.Object) error {
	var relRefs []metadata.ObjectRef
	for _, ref := range refs {
		if ref.Kind == "table" || ref.Kind == "view" {
			relRefs = append(relRefs, ref)
		}
	}
	if len(relRefs) == 0 {
		return nil
	}
	cols, err := jsonColumns(ctx, db, relRefs)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	for i := range objs {
		if objs[i].Relational == nil {
			continue
		}
		prefix := objs[i].Ref.Scope.Name("database") + "\x00" + objs[i].Ref.Name + "\x00"
		for c := range objs[i].Relational.Columns {
			col := &objs[i].Relational.Columns[c]
			if cols[prefix+col.Name] && strings.EqualFold(col.DataType, "longtext") {
				col.DataType = "json"
			}
		}
	}
	return nil
}
