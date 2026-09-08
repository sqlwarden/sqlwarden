package sqlserver

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.RelationshipInspector = (*Driver)(nil)

// InspectRelationshipsInScope reports foreign-key edges for the schema named
// in scope. sys.foreign_keys/sys.foreign_key_columns are always scoped to the
// connection's current database (see CatalogTables), so only the schema
// level needs filtering here.
func (d *Driver) InspectRelationshipsInScope(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	namespace := scope.Name("schema")
	const q = `
SELECT s.name, o.name, fk.name, col.name, rs.name, ro.name, rcol.name
FROM sys.foreign_keys fk
JOIN sys.foreign_key_columns fkc ON fkc.constraint_object_id = fk.object_id
JOIN sys.objects o ON o.object_id = fk.parent_object_id
JOIN sys.schemas s ON s.schema_id = o.schema_id
JOIN sys.columns col ON col.object_id = fkc.parent_object_id AND col.column_id = fkc.parent_column_id
JOIN sys.objects ro ON ro.object_id = fk.referenced_object_id
JOIN sys.schemas rs ON rs.schema_id = ro.schema_id
JOIN sys.columns rcol ON rcol.object_id = fkc.referenced_object_id AND rcol.column_id = fkc.referenced_column_id
WHERE s.name = @p1
ORDER BY s.name, o.name, fk.name, fkc.constraint_column_id`
	rows, err := d.db.QueryContext(ctx, q, namespace)
	if err != nil {
		return nil, fmt.Errorf("sqlserver: relationships: %w", err)
	}
	defer rows.Close()

	graph := &metadata.RelationshipGraph{Scope: scope}
	index := map[string]int{} // constraint key -> position in graph.Relationships
	for rows.Next() {
		var ns, tbl, name, col, refNs, refTbl, refCol string
		if err := rows.Scan(&ns, &tbl, &name, &col, &refNs, &refTbl, &refCol); err != nil {
			return nil, fmt.Errorf("sqlserver: relationships scan: %w", err)
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
		return nil, fmt.Errorf("sqlserver: relationships rows: %w", err)
	}
	return graph, nil
}
