package mariadb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

func mariadbPairFilter(refs []metadata.ObjectRef) (string, []any) {
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
// MariaDB's own SEQUENCE definition (start/increment/min/max/cache/cycle) is
// served lazily through InspectDefinition's "SHOW CREATE SEQUENCE", matching
// how table DDL and view/routine bodies are handled elsewhere in this engine
// family — this only surfaces the cheap information_schema.tables detail.
func SequenceObjects(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	pairs, args := mariadbPairFilter(refs)
	q := `
SELECT table_schema, table_name, engine
FROM information_schema.tables
WHERE (table_schema, table_name) IN (` + pairs + `)
ORDER BY table_schema, table_name`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("mariadb: sequence detail: %w", err)
	}
	defer rows.Close()
	var out []metadata.Object
	for rows.Next() {
		var ns, name string
		var eng sql.NullString
		if err := rows.Scan(&ns, &name, &eng); err != nil {
			return nil, fmt.Errorf("mariadb: sequence detail scan: %w", err)
		}
		out = append(out, metadata.Object{
			Ref: mariadbRequestedRef(refs, ns, name, "sequence"),
			Descriptors: []metadata.Descriptor{
				{Kind: "fields", Title: "Sequence", Fields: []metadata.Field{{Name: "Engine", Value: eng.String}}},
			},
		})
	}
	return out, rows.Err()
}

func mariadbRequestedRef(refs []metadata.ObjectRef, database, name, kind string) metadata.ObjectRef {
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
