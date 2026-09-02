package oracle

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.RelationshipInspector = (*oracleDriver)(nil)

const oracleRelationshipsQuery = `
SELECT c.table_name, c.constraint_name, cc.column_name,
       rc.owner AS ref_owner, rc.table_name AS ref_table, rcc.column_name AS ref_column
FROM all_constraints c
JOIN all_cons_columns cc
  ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
JOIN all_constraints rc
  ON rc.owner = c.r_owner AND rc.constraint_name = c.r_constraint_name
JOIN all_cons_columns rcc
  ON rcc.owner = rc.owner AND rcc.constraint_name = rc.constraint_name AND rcc.position = cc.position
WHERE c.constraint_type = 'R' AND c.owner = :1
ORDER BY c.table_name, c.constraint_name, cc.position`

func (d *oracleDriver) InspectRelationshipsInScope(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	owner := scope.Name("schema")
	rows, err := d.db.QueryContext(ctx, oracleRelationshipsQuery, owner)
	if err != nil {
		return nil, fmt.Errorf("oracle: relationships: %w", err)
	}
	defer rows.Close()

	graph := &metadata.RelationshipGraph{Scope: scope}
	index := map[string]int{}
	for rows.Next() {
		var table, name, column, refOwner, refTable, refColumn string
		if err := rows.Scan(&table, &name, &column, &refOwner, &refTable, &refColumn); err != nil {
			return nil, fmt.Errorf("oracle: relationships scan: %w", err)
		}
		key := table + "\x00" + name
		pos, ok := index[key]
		if !ok {
			graph.Relationships = append(graph.Relationships, metadata.Relationship{
				Kind:   "foreign_key",
				Name:   name,
				Source: metadata.ObjectRef{Scope: scope, Kind: "table", Name: table},
				References: metadata.ObjectRef{
					Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: refOwner}),
					Kind:  "table",
					Name:  refTable,
				},
			})
			pos = len(graph.Relationships) - 1
			index[key] = pos
		}
		graph.Relationships[pos].Columns = append(graph.Relationships[pos].Columns, column)
		graph.Relationships[pos].ReferencedColumns = append(graph.Relationships[pos].ReferencedColumns, refColumn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: relationships rows: %w", err)
	}
	return graph, nil
}
