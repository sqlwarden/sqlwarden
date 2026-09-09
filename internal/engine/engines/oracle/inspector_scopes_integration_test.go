//go:build integration

package oracle

import (
	"context"
	"database/sql"
	"net/url"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestOracleVisibleSchemasAndCrossSchemaInspection(t *testing.T) {
	ctx := context.Background()
	adminURL, err := url.Parse(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	adminURL.User = url.UserPassword("system", "warden_sys")
	admin, err := sql.Open("oracle", adminURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	exec := func(query string) {
		t.Helper()
		if _, err := admin.ExecContext(ctx, query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	for _, name := range []string{"BROWSE_LOGIN", "BROWSE_OTHER", "BROWSE_HIDDEN", "BROWSE_EMPTY"} {
		exec("CREATE USER " + name + " IDENTIFIED BY browse_test")
		t.Cleanup(func() {
			db, err := sql.Open("oracle", adminURL.String())
			if err == nil {
				defer db.Close()
				_, _ = db.ExecContext(ctx, "DROP USER "+name+" CASCADE")
			}
		})
	}
	exec("GRANT CREATE SESSION TO BROWSE_LOGIN")
	exec("ALTER USER BROWSE_OTHER QUOTA UNLIMITED ON USERS")
	exec("CREATE TABLE BROWSE_OTHER.VISIBLE_TABLE (OTHER_ID NUMBER)")
	exec("GRANT SELECT ON BROWSE_OTHER.VISIBLE_TABLE TO BROWSE_LOGIN")
	exec("ALTER USER BROWSE_HIDDEN QUOTA UNLIMITED ON USERS")
	exec("CREATE TABLE BROWSE_HIDDEN.PRIVATE_TABLE (PRIVATE_ID NUMBER)")
	loginURL := *adminURL
	loginURL.User = url.UserPassword("BROWSE_LOGIN", "browse_test")
	d := &oracleDriver{}
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: loginURL.String(), Driver: "oracle"}); err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	directory, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]metadata.ScopeNode{}
	for _, node := range directory.Roots {
		found[node.Path.Name("schema")] = node
	}
	if node, ok := found["BROWSE_LOGIN"]; !ok || node.Lazy {
		t.Fatal("empty login/current schema must be loaded and visible")
	}
	if node, ok := found["BROWSE_OTHER"]; !ok || !node.Lazy || len(node.Groups) != 0 {
		t.Fatal("accessible schema must be listed lazily")
	}
	for _, name := range []string{"BROWSE_HIDDEN", "BROWSE_EMPTY"} {
		node, ok := found[name]
		if !ok || !node.Lazy || len(node.Groups) != 0 {
			t.Fatalf("%s must be visible and unloaded regardless of object grants", name)
		}
		expanded, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{Root: node.Path})
		if err != nil {
			t.Fatal(err)
		}
		if len(expanded.Roots) != 1 || expanded.Roots[0].Lazy || len(expanded.ObjectRefs()) != 0 {
			t.Fatalf("%s exposed inaccessible objects: %+v", name, expanded)
		}
	}
	scopes, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	discovered := map[string]bool{}
	for _, path := range scopes.Scopes {
		discovered[path.Name("schema")] = true
	}
	if !discovered["BROWSE_HIDDEN"] || !discovered["BROWSE_EMPTY"] || discovered["SYS"] {
		t.Fatalf("connection discovery must include visible non-system users: %+v", scopes)
	}
	if node, ok := found["SYS"]; !ok || !node.System {
		t.Fatal("system classification missing")
	}
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "BROWSE_OTHER"})
	loaded, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{Root: scope})
	if err != nil {
		t.Fatal(err)
	}
	ref := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "VISIBLE_TABLE"}
	seen := false
	for _, got := range loaded.ObjectRefs() {
		seen = seen || got == ref
	}
	if !seen || len(loaded.Roots) != 1 || loaded.Roots[0].Lazy {
		t.Fatal("expanded schema not loaded")
	}
	current, err := d.currentSchema(ctx)
	if err != nil || current != "BROWSE_LOGIN" {
		t.Fatalf("browsing changed current schema: %s %v", current, err)
	}
	// Every query uses the same session here so ALTER SESSION reliably exercises
	// the USER_* shortcut independently of the connection's configured default.
	d.db.SetMaxOpenConns(1)
	if _, err := d.db.ExecContext(ctx, "ALTER SESSION SET CURRENT_SCHEMA = BROWSE_OTHER"); err != nil {
		t.Fatal(err)
	}
	objects, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Relational == nil || len(objects[0].Relational.Columns) != 1 || objects[0].Relational.Columns[0].Name != "OTHER_ID" {
		t.Fatalf("cross-schema columns: %+v", objects)
	}
}
