package schema

import (
	"context"
	"fmt"
	"testing"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func seedCompletionCache(b *testing.B, store *fakeStore, conn Connection, schemas, tables, detailed, columns int) metadata.Tree {
	b.Helper()
	tree := newFakeCatalog().Tree()
	n := newTestNavigator(store)
	ctx := context.Background()
	save := func(parent metadata.ScopePath, folder string, items []metadata.Child) {
		row, err := encodeListing(conn.ID, Listing{Parent: parent, Folder: folder, Items: items, FetchedAt: fixedNow()})
		if err != nil {
			b.Fatal(err)
		}
		if err := store.UpsertSchemaListings(ctx, []database.SchemaListing{row}); err != nil {
			b.Fatal(err)
		}
	}
	save("", "databases", []metadata.Child{{Kind: "database", Name: "app"}})
	var schemaItems []metadata.Child
	for s := range schemas {
		schemaItems = append(schemaItems, metadata.Child{Kind: "schema", Name: fmt.Sprintf("schema_%03d", s)})
	}
	save(dbPath("app"), "schemas", schemaItems)
	var objects []metadata.Object
	for s := range schemas {
		schemaName := fmt.Sprintf("schema_%03d", s)
		var tableItems []metadata.Child
		for t := range tables {
			tableName := fmt.Sprintf("table_%04d", t)
			tableItems = append(tableItems, metadata.Child{Kind: "table", Name: tableName})
			if s == 0 && t < detailed {
				cols := make([]metadata.Column, columns)
				colItems := make([]metadata.Child, columns)
				for c := range columns {
					cols[c] = metadata.Column{Name: fmt.Sprintf("column_%02d", c), DataType: "text", Nullable: true, Ordinal: c + 1}
					colItems[c] = metadata.Child{Kind: "column", Name: cols[c].Name, Attributes: map[string]any{"data_type": "text", "ordinal": c + 1}}
				}
				save(tablePathOf("app", schemaName, tableName), "columns", colItems)
				objects = append(objects, metadata.Object{
					Ref:        metadata.ObjectRef{Scope: schemaPathOf("app", schemaName), Kind: "table", Name: tableName},
					Relational: &metadata.RelationalDetail{Columns: cols},
				})
			}
		}
		save(schemaPathOf("app", schemaName), "tables", tableItems)
	}
	if err := n.saveObjects(ctx, conn, objects); err != nil {
		b.Fatal(err)
	}
	return tree
}

func BenchmarkCompletionView(b *testing.B) {
	for _, size := range []struct{ schemas, tables, detailed, columns int }{
		{50, 200, 20, 30},
		{50, 200, 500, 30},
	} {
		b.Run(fmt.Sprintf("schemas=%d/tables=%d/detailed=%d", size.schemas, size.tables, size.detailed), func(b *testing.B) {
			store := newFakeStore()
			conn := Connection{ID: 1, Persistent: true, ShowAllDatabases: true, DefaultScope: schemaPathOf("app", "schema_000")}
			tree := seedCompletionCache(b, store, conn, size.schemas, size.tables, size.detailed, size.columns)
			n := newTestNavigator(store)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := n.CompletionView(ctx, conn, tree, nil); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
