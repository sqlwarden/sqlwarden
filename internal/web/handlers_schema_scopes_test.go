package web

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

type browseSchemaDriver struct{ schemaFakeDriver }

func (browseSchemaDriver) SchemaSpec() metadata.SchemaSpec {
	spec := schemaFakeDriver{}.SchemaSpec()
	spec.BrowseScopes = true
	return spec
}
func (browseSchemaDriver) InspectDirectory(_ context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	return &metadata.Directory{Roots: []metadata.ScopeNode{{Path: opts.Root, Groups: []metadata.ObjectGroup{{Kind: "table", Objects: []metadata.ObjectRef{{Scope: opts.Root, Kind: "table", Name: "report"}}}}}}}, nil
}

func TestSchemaScopeDirectorySessionAndCompletion(t *testing.T) {
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "browse"), "Browse", "Browse Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	sess := openSchemaSession(t, app, owner.ID, conn.ID, browseSchemaDriver{})
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "REPORTING"})
	endpoint := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10)) + "/schema/directory?scope=" + schemaScopeParam(scope)
	req := newAuthRequest(t, http.MethodGet, endpoint, nil, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("scope request: %d %+v", res.StatusCode, res.BodyFields)
	}
	// A warmed cache must not let ephemeral requests skip session validation.
	req = newAuthRequest(t, http.MethodGet, endpoint, nil, tok)
	res = send(t, req, app.routes())
	if res.StatusCode == http.StatusOK {
		t.Fatal("scope cache bypassed session validation")
	}

	connID := strconv.FormatInt(conn.ID, 10)
	base := &metadata.Directory{Roots: []metadata.ScopeNode{{Path: scope, Lazy: true}}}
	merged, objects, version := app.completionWithCachedScopes(connID, base, nil, "snapshot")
	index := projectCompletionIndex(merged, objects, version)
	if len(index.Objects) != 1 || index.Objects[0].Name != "report" {
		t.Fatalf("directory-only names missing: %+v", index)
	}
	if len(index.Columns) != 0 {
		t.Fatal("unexpected eager detail inspection")
	}
	if _, err := app.schemaService.Objects(context.Background(), connID, merged.ObjectRefs(), browseSchemaDriver{}); err != nil {
		t.Fatal(err)
	}
	merged, objects, next := app.completionWithCachedScopes(connID, base, nil, "snapshot")
	index = projectCompletionIndex(merged, objects, next)
	if next == version || len(index.Columns) != 1 || len(index.Objects) != 1 {
		t.Fatalf("details did not update completion: %+v", index)
	}
	if !base.Roots[0].Lazy {
		t.Fatal("snapshot was changed")
	}
}

func TestPersistentExpandedScopeServesCachedMetadataWithoutSession(t *testing.T) {
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "browse-snapshot"), "Browse Snapshot", "Browse Snapshot Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Schema WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn")
	connID := strconv.FormatInt(conn.ID, 10)
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "REPORTING"})
	ctx := context.Background()
	base := &metadata.Directory{Engine: "sqlite", Roots: []metadata.ScopeNode{{Path: scope, Lazy: true}}}
	snapshot, err := app.schemaSnapshots.Begin(ctx, conn.ID, &org.ID, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(ctx, snapshot.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := app.schemaService.DirectoryInScope(ctx, connID, scope, browseSchemaDriver{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.schemaService.Objects(ctx, connID, loaded.ObjectRefs(), browseSchemaDriver{}); err != nil {
		t.Fatal(err)
	}
	endpoint := orgConnectionURL(org.Slug, ws.ID, envID, connID)
	req := newAuthRequest(t, http.MethodGet, endpoint+"/schema/directory?scope="+schemaScopeParam(scope), nil, tok)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cached scope: %d %+v", res.StatusCode, res.BodyFields)
	}
	req = newAuthRequest(t, http.MethodPost, endpoint+"/schema/objects", map[string]any{"refs": loaded.ObjectRefs()}, tok)
	res = send(t, req, app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cached objects: %d %+v", res.StatusCode, res.BodyFields)
	}
	objects, ok := res.BodyFields["objects"].([]any)
	if !ok || len(objects) != 1 {
		t.Fatalf("missing expanded objects: %+v", res.BodyFields)
	}
	_, unchanged, found, err := app.schemaSnapshots.Active(ctx, conn.ID)
	if err != nil || !found || !unchanged.Roots[0].Lazy {
		t.Fatalf("snapshot changed: %v", err)
	}
}
