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

	var scanned []oracleForeignKeyRow
	for rows.Next() {
		var r oracleForeignKeyRow
		if err := rows.Scan(&r.Table, &r.Name, &r.Column, &r.RefOwner, &r.RefTable, &r.RefColumn); err != nil {
			return nil, fmt.Errorf("oracle: relationships scan: %w", err)
		}
		scanned = append(scanned, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: relationships rows: %w", err)
	}

	return &metadata.RelationshipGraph{
		Scope:         scope,
		Relationships: foldOracleForeignKeys(scope, scanned),
	}, nil
}

// oracleForeignKeyRow is one (constraint, column) row of oracleRelationshipsQuery,
// already ordered by (table, constraint, position).
type oracleForeignKeyRow struct {
	Table     string
	Name      string
	Column    string
	RefOwner  string
	RefTable  string
	RefColumn string
}

// foldOracleForeignKeys groups the per-column rows into one Relationship per
// constraint, accumulating Columns/ReferencedColumns in the row order (which the
// query fixes to constraint column position).
func foldOracleForeignKeys(scope metadata.ScopePath, rows []oracleForeignKeyRow) []metadata.Relationship {
	var out []metadata.Relationship
	index := map[string]int{}
	for _, r := range rows {
		key := r.Table + "\x00" + r.Name
		pos, ok := index[key]
		if !ok {
			out = append(out, metadata.Relationship{
				Kind:   "foreign_key",
				Name:   r.Name,
				Source: metadata.ObjectRef{Scope: scope, Kind: "table", Name: r.Table},
				References: metadata.ObjectRef{
					Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: r.RefOwner}),
					Kind:  "table",
					Name:  r.RefTable,
				},
			})
			pos = len(out) - 1
			index[key] = pos
		}
		out[pos].Columns = append(out[pos].Columns, r.Column)
		out[pos].ReferencedColumns = append(out[pos].ReferencedColumns, r.RefColumn)
	}
	return out
}
