package mysql

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.RelationshipInspector = (*Driver)(nil)

func (d *Driver) InspectRelationshipsInScope(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	namespace := scope.Name("database")
	q := `
SELECT table_schema, table_name, constraint_name, column_name,
       referenced_table_schema, referenced_table_name, referenced_column_name
FROM information_schema.key_column_usage
WHERE referenced_table_name IS NOT NULL AND table_schema = ?
ORDER BY table_schema, table_name, constraint_name, ordinal_position`
	rows, err := d.db.QueryContext(ctx, q, namespace)
	if err != nil {
		return nil, fmt.Errorf("mysql: relationships: %w", err)
	}
	defer rows.Close()

	graph := &metadata.RelationshipGraph{Scope: scope}
	index := map[string]int{}
	for rows.Next() {
		var ns, tbl, name, col, refNs, refTbl, refCol string
		if err := rows.Scan(&ns, &tbl, &name, &col, &refNs, &refTbl, &refCol); err != nil {
			return nil, fmt.Errorf("mysql: relationships scan: %w", err)
		}
		key := ns + "\x00" + tbl + "\x00" + name
		pos, ok := index[key]
		if !ok {
			graph.Relationships = append(graph.Relationships, metadata.Relationship{
				Kind:       "foreign_key",
				Name:       name,
				Source:     metadata.ObjectRef{Scope: scope, Kind: "table", Name: tbl},
				References: metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: refNs}), Kind: "table", Name: refTbl},
			})
			pos = len(graph.Relationships) - 1
			index[key] = pos
		}
		graph.Relationships[pos].Columns = append(graph.Relationships[pos].Columns, col)
		graph.Relationships[pos].ReferencedColumns = append(graph.Relationships[pos].ReferencedColumns, refCol)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: relationships rows: %w", err)
	}
	return graph, nil
}
