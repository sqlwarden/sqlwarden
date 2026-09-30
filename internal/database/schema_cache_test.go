package database

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine/metadata"
)

func seedSchemaCacheConnection(t *testing.T, db *DB) int64 {
	t.Helper()
	ctx := context.Background()
	org, err := db.InsertOrg(ctx, "schema-cache-org", "Schema Cache Org")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(ctx, ws.ID, nil, "cache", "postgres", "dsn", "open")
	if err != nil {
		t.Fatal(err)
	}
	return conn.ID
}

func listingPaths(rows []SchemaListing) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ParentPath+"#"+row.Folder)
	}
	slices.Sort(out)
	return out
}

func TestSchemaListingsSubtreeRespectsBoundariesAndWildcards(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			connID := seedSchemaCacheConnection(t, db)
			now := time.Now().UTC()
			underscoreRoot := string(metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "a_b"}))
			underscoreFalseDescendant := string(metadata.NewScopePath(
				metadata.ScopeSegment{Kind: "database", Name: "axb"},
				metadata.ScopeSegment{Kind: "schema", Name: "s"},
			))
			percentRoot := string(metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "a%b"}))
			percentFalseDescendant := string(metadata.NewScopePath(
				metadata.ScopeSegment{Kind: "database", Name: "aQQ25b"},
				metadata.ScopeSegment{Kind: "schema", Name: "s"},
			))
			rows := []SchemaListing{
				{ConnectionID: connID, ParentPath: "", Folder: "databases", ChildrenData: []byte("r"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: "database=a", Folder: "schemas", ChildrenData: []byte("a"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: "database=a/schema=s_1", Folder: "tables", ChildrenData: []byte("a1"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: "database=ab", Folder: "schemas", ChildrenData: []byte("ab"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: percentRoot, Folder: "schemas", ChildrenData: []byte("pct"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: "database=axb", Folder: "schemas", ChildrenData: []byte("axb"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: underscoreRoot, Folder: "schemas", ChildrenData: []byte("a_b"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: underscoreFalseDescendant, Folder: "tables", ChildrenData: []byte("underscore-false"), FetchedAt: now},
				{ConnectionID: connID, ParentPath: percentFalseDescendant, Folder: "tables", ChildrenData: []byte("percent-false"), FetchedAt: now},
			}
			if err := db.UpsertSchemaListings(ctx, rows); err != nil {
				t.Fatal(err)
			}

			got, err := db.SchemaListingsWithin(ctx, connID, "database=a")
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"database=a#schemas", "database=a/schema=s_1#tables"}
			if !slices.Equal(listingPaths(got), want) {
				t.Fatalf("within database=a = %v, want %v", listingPaths(got), want)
			}

			got, _ = db.SchemaListingsWithin(ctx, connID, underscoreRoot)
			if !slices.Equal(listingPaths(got), []string{underscoreRoot + "#schemas"}) {
				t.Fatalf("underscore must not act as a wildcard: %v", listingPaths(got))
			}
			got, _ = db.SchemaListingsWithin(ctx, connID, percentRoot)
			if !slices.Equal(listingPaths(got), []string{percentRoot + "#schemas"}) {
				t.Fatalf("percent must not act as a wildcard: %v", listingPaths(got))
			}

			if err := db.DeleteSchemaListingsWithin(ctx, connID, underscoreRoot); err != nil {
				t.Fatal(err)
			}
			if _, found, err := db.SchemaListing(ctx, connID, underscoreFalseDescendant, "tables"); err != nil || !found {
				t.Fatalf("underscore false-match descendant deleted: found=%v err=%v", found, err)
			}
			if err := db.DeleteSchemaListingsWithin(ctx, connID, percentRoot); err != nil {
				t.Fatal(err)
			}
			if _, found, err := db.SchemaListing(ctx, connID, percentFalseDescendant, "tables"); err != nil || !found {
				t.Fatalf("percent false-match descendant deleted: found=%v err=%v", found, err)
			}

			if err := db.DeleteSchemaListingsWithin(ctx, connID, "database=a"); err != nil {
				t.Fatal(err)
			}
			got, _ = db.SchemaListingsWithin(ctx, connID, "")
			want = []string{
				"#databases",
				percentFalseDescendant + "#tables",
				underscoreFalseDescendant + "#tables",
				"database=ab#schemas",
				"database=axb#schemas",
			}
			slices.Sort(want)
			if !slices.Equal(listingPaths(got), want) {
				t.Fatalf("after delete = %v, want %v", listingPaths(got), want)
			}
		})
	}
}

func TestSchemaListingUpsertReplacesData(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			connID := seedSchemaCacheConnection(t, db)
			first := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
			second := first.Add(time.Hour)
			_ = db.UpsertSchemaListings(ctx, []SchemaListing{{ConnectionID: connID, ParentPath: "database=a", Folder: "schemas", ChildrenData: []byte("old"), FetchedAt: first}})
			_ = db.UpsertSchemaListings(ctx, []SchemaListing{{ConnectionID: connID, ParentPath: "database=a", Folder: "schemas", ChildrenData: []byte("new"), FetchedAt: second}})
			row, found, err := db.SchemaListing(ctx, connID, "database=a", "schemas")
			if err != nil || !found {
				t.Fatalf("SchemaListing = %v, %v", found, err)
			}
			if string(row.ChildrenData) != "new" || !row.FetchedAt.Equal(second) {
				t.Fatalf("row = %q at %v", row.ChildrenData, row.FetchedAt)
			}
			if _, found, _ := db.SchemaListing(ctx, connID, "database=a", "extensions"); found {
				t.Fatal("unexpected listing")
			}
		})
	}
}

func TestSchemaObjectsAndRelationshipsInvalidation(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			connID := seedSchemaCacheConnection(t, db)
			now := time.Now().UTC()
			objects := []SchemaObject{
				{ConnectionID: connID, Scope: "database=a/schema=s", Kind: "table", Name: "t1", ObjectData: []byte("t1"), FetchedAt: now},
				{ConnectionID: connID, Scope: "database=a/schema=s", Kind: "table", Name: "t2", ObjectData: []byte("t2"), FetchedAt: now},
				{ConnectionID: connID, Scope: "database=a/schema=other", Kind: "table", Name: "t3", ObjectData: []byte("t3"), FetchedAt: now},
			}
			if err := db.UpsertSchemaObjects(ctx, objects); err != nil {
				t.Fatal(err)
			}
			got, err := db.SchemaObjects(ctx, connID, []SchemaObjectKey{{Scope: "database=a/schema=s", Kind: "table", Name: "t1"}, {Scope: "database=a/schema=s", Kind: "table", Name: "missing"}})
			if err != nil || len(got) != 1 || got[0].Name != "t1" {
				t.Fatalf("SchemaObjects = %v, %v", got, err)
			}
			if err := db.DeleteSchemaObjects(ctx, connID, "database=a/schema=s/table=t1", []SchemaObjectKey{{Scope: "database=a/schema=s", Kind: "table", Name: "t1"}}); err != nil {
				t.Fatal(err)
			}
			all, _ := db.AllSchemaObjects(ctx, connID)
			if len(all) != 2 {
				t.Fatalf("exact delete should remove only t1: %v", all)
			}
			if err := db.DeleteSchemaObjects(ctx, connID, "database=a/schema=s", nil); err != nil {
				t.Fatal(err)
			}
			all, _ = db.AllSchemaObjects(ctx, connID)
			if len(all) != 1 || all[0].Name != "t3" {
				t.Fatalf("subtree delete should keep t3 only: %v", all)
			}

			_ = db.UpsertSchemaRelationship(ctx, SchemaRelationship{ConnectionID: connID, Scope: "database=a/schema=s", Data: []byte("g"), FetchedAt: now})
			_ = db.UpsertSchemaRelationship(ctx, SchemaRelationship{ConnectionID: connID, Scope: "database=a/schema=other", Data: []byte("g2"), FetchedAt: now})
			if err := db.DeleteSchemaRelationships(ctx, connID, "database=a/schema=s/table=t1", []string{"", "database=a", "database=a/schema=s"}); err != nil {
				t.Fatal(err)
			}
			if _, found, _ := db.SchemaRelationship(ctx, connID, "database=a/schema=s"); found {
				t.Fatal("ancestor relationship graph must be invalidated")
			}
			if _, found, _ := db.SchemaRelationship(ctx, connID, "database=a/schema=other"); !found {
				t.Fatal("unrelated relationship graph must survive")
			}

			_ = db.UpsertSchemaListings(ctx, []SchemaListing{{ConnectionID: connID, ParentPath: "", Folder: "databases", ChildrenData: []byte("x"), FetchedAt: now}})
			if err := db.DeleteSchemaCache(ctx, connID); err != nil {
				t.Fatal(err)
			}
			listings, _ := db.SchemaListingsWithin(ctx, connID, "")
			all, _ = db.AllSchemaObjects(ctx, connID)
			if len(listings) != 0 || len(all) != 0 {
				t.Fatal("DeleteSchemaCache must clear every table")
			}
		})
	}
}
