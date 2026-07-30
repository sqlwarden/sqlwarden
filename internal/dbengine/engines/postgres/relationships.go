package postgres

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/dbengine/metadata"
)

var _ metadata.RelationshipInspector = (*postgresDriver)(nil)

func (d *postgresDriver) InspectRelationshipsInScope(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	namespace := scope.Name("schema")
	q := `
SELECT tc.table_schema, tc.table_name, tc.constraint_name, kcu.column_name,
       ccu.table_schema, ccu.table_name, ccu.column_name
FROM information_schema.table_constraints tc
JOIN information_schema.key_column_usage kcu
  ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
JOIN information_schema.constraint_column_usage ccu
  ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = $1
ORDER BY tc.table_schema, tc.table_name, tc.constraint_name, kcu.ordinal_position`
	rows, err := d.db.QueryContext(ctx, q, namespace)
	if err != nil {
		return nil, fmt.Errorf("postgres: relationships: %w", err)
	}
	defer rows.Close()

	graph := &metadata.RelationshipGraph{Scope: scope}
	index := map[string]int{} // constraint key -> position in graph.Relationships
	for rows.Next() {
		var ns, tbl, name, col, refNs, refTbl, refCol string
		if err := rows.Scan(&ns, &tbl, &name, &col, &refNs, &refTbl, &refCol); err != nil {
			return nil, fmt.Errorf("postgres: relationships scan: %w", err)
		}
		key := ns + "\x00" + tbl + "\x00" + name
		pos, ok := index[key]
		if !ok {
			graph.Relationships = append(graph.Relationships, metadata.Relationship{
				Kind:       "foreign_key",
				Name:       name,
				Source:     metadata.ObjectRef{Scope: scope.With("schema", ns), Kind: "table", Name: tbl},
				References: metadata.ObjectRef{Scope: scope.With("schema", refNs), Kind: "table", Name: refTbl},
			})
			pos = len(graph.Relationships) - 1
			index[key] = pos
		}
		graph.Relationships[pos].Columns = append(graph.Relationships[pos].Columns, col)
		graph.Relationships[pos].ReferencedColumns = append(graph.Relationships[pos].ReferencedColumns, refCol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: relationships rows: %w", err)
	}
	return graph, nil
}
