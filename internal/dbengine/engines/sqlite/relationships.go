package sqlite

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/dbengine/schema"
)

var _ schema.RelationshipInspector = (*sqliteDriver)(nil)

// InspectRelationships returns every foreign-key edge in a namespace. SQLite has
// no namespace-wide FK view, so it lists the namespace's tables and runs
// PRAGMA foreign_key_list per table, grouping rows by fk id.
func (d *sqliteDriver) InspectRelationships(ctx context.Context, namespace string) (*schema.RelationshipGraph, error) {
	prefix := sqliteQuoteIdent(namespace)
	// SQLite cannot bind identifiers; sqliteQuoteIdent escapes the namespace.
	tableRows, err := d.db.QueryContext(ctx,
		// codeql[go/sql-injection]
		fmt.Sprintf(`SELECT name FROM %s.sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%%' ORDER BY name`, prefix))
	if err != nil {
		return nil, fmt.Errorf("sqlite: relationships tables: %w", err)
	}
	var tables []string
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			tableRows.Close()
			return nil, fmt.Errorf("sqlite: relationships tables scan: %w", err)
		}
		tables = append(tables, name)
	}
	if err := tableRows.Err(); err != nil {
		tableRows.Close()
		return nil, fmt.Errorf("sqlite: relationships tables rows: %w", err)
	}
	tableRows.Close()

	graph := &schema.RelationshipGraph{Namespace: namespace}
	for _, tbl := range tables {
		// codeql[go/sql-injection]
		fkRows, err := d.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA %s.foreign_key_list(%s)`, prefix, sqliteQuoteIdent(tbl)))
		if err != nil {
			return nil, fmt.Errorf("sqlite: relationships fk: %w", err)
		}
		perID := map[int]int{} // fk id -> position
		for fkRows.Next() {
			var id, seq int
			var refTbl, fromCol, toCol, onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &refTbl, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
				fkRows.Close()
				return nil, fmt.Errorf("sqlite: relationships fk scan: %w", err)
			}
			pos, ok := perID[id]
			if !ok {
				graph.Relationships = append(graph.Relationships, schema.Relationship{
					Name:       fmt.Sprintf("%s_fk_%d", tbl, id),
					Source:     schema.ObjectRef{Namespace: namespace, Kind: "table", Name: tbl},
					References: schema.ObjectRef{Namespace: namespace, Kind: "table", Name: refTbl},
				})
				pos = len(graph.Relationships) - 1
				perID[id] = pos
			}
			graph.Relationships[pos].Columns = append(graph.Relationships[pos].Columns, fromCol)
			graph.Relationships[pos].ReferencedColumns = append(graph.Relationships[pos].ReferencedColumns, toCol)
		}
		if err := fkRows.Err(); err != nil {
			fkRows.Close()
			return nil, fmt.Errorf("sqlite: relationships fk rows: %w", err)
		}
		fkRows.Close()
	}
	return graph, nil
}
