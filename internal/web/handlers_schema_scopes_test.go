package web

import (
	"context"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
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
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
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
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Schema Conn", "open")
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

// TestLiveSchemaObjectsWriteThroughPersistsIntoActiveSnapshot proves that a
// genuine cache-miss object fetch in persistent mode (as opposed to the
// pre-warmed-cache scenario above) writes the fetched detail into the
// connection's active snapshot, so a later request reads the same detail
// without re-inspecting the driver.
func TestLiveSchemaObjectsWriteThroughPersistsIntoActiveSnapshot(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-writethrough"), "Snapshot WriteThrough", "Snapshot WriteThrough Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	updateInstanceSettingsForTest(t, app, func(s *database.InstanceSettings) {
		s.SchemaLazyThreshold = 1
	})

	dsn := filepath.Join(t.TempDir(), "target.db")
	driver, err := engine.New("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Connect(context.Background(), engine.ConnectionConfig{DSN: dsn}); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Execute(context.Background(), "CREATE TABLE widgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Execute(context.Background(), "CREATE TABLE gadgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}

	created := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "Target", "driver": "sqlite", "dsn": dsn}, tok), app.routes())
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create target connection: status=%d body=%s", created.StatusCode, created.BodyBytes)
	}
	connectionID := int64(created.BodyFields["id"].(float64))

	refreshRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connectionID, 10))+"/schema/refresh", nil, tok), app.routes())
	if refreshRes.StatusCode != http.StatusOK {
		t.Fatalf("schema refresh: status=%d body=%s", refreshRes.StatusCode, refreshRes.BodyBytes)
	}

	ctx := context.Background()
	_, directory, found, err := app.schemaSnapshots.Active(ctx, connectionID)
	if err != nil || !found {
		t.Fatalf("expected active snapshot: found=%v err=%v", found, err)
	}
	if !directory.Roots[0].Lazy {
		t.Fatalf("expected the scope with 2 objects over threshold 1 to be marked lazy: %+v", directory.Roots[0])
	}
	var ref metadata.ObjectRef
	for _, group := range directory.Roots[0].Groups {
		for _, candidate := range group.Objects {
			if candidate.Name == "widgets" {
				ref = candidate
			}
		}
	}
	if ref.Name != "widgets" {
		t.Fatalf("expected widgets ref in directory: %+v", directory.Roots[0])
	}

	// A live session is required to open the temporary target connection this
	// fetch needs; the caller isn't the one that had the DB connection, the
	// session just proves they're actively connected in the IDE.
	sess, _, err := testConnectionManager(t, app).GetOrCreate(
		strconv.FormatInt(owner.ID, 10),
		strconv.FormatInt(connectionID, 10),
		func() (engine.Driver, func(), error) { return schemaFakeDriver{}, nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	endpoint := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connectionID, 10))
	req := newAuthRequest(t, http.MethodPost, endpoint+"/schema/objects", map[string]any{"refs": []metadata.ObjectRef{ref}}, tok)
	req.Header.Set("X-Warden-Session", sess.ID)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("live objects: status=%d body=%s", res.StatusCode, res.BodyBytes)
	}
	objects, ok := res.BodyFields["objects"].([]any)
	if !ok || len(objects) != 1 {
		t.Fatalf("expected one live-fetched object, got %+v", res.BodyFields)
	}
	if res.BodyFields["pending_connection"] != nil {
		t.Fatalf("did not expect pending_connection when a live session fetched the object: %+v", res.BodyFields)
	}

	snapshot, _, found, err := app.schemaSnapshots.Active(ctx, connectionID)
	if err != nil || !found {
		t.Fatalf("expected active snapshot to remain: found=%v err=%v", found, err)
	}
	stored, err := app.schemaSnapshots.Objects(ctx, snapshot.ID, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected the live-fetched object to be written into the snapshot, got %d", len(stored))
	}
}

// TestLiveSchemaObjectsWithoutSessionReturnsPendingConnection proves that
// expanding an uncached, lazily-scoped object with no active session does not
// open a temporary connection to the target database — it reports the ref as
// pending connection instead, so the caller can prompt to connect.
func TestLiveSchemaObjectsWithoutSessionReturnsPendingConnection(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-pending"), "Snapshot Pending", "Snapshot Pending Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	updateInstanceSettingsForTest(t, app, func(s *database.InstanceSettings) {
		s.SchemaLazyThreshold = 1
	})

	dsn := filepath.Join(t.TempDir(), "target.db")
	driver, err := engine.New("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Connect(context.Background(), engine.ConnectionConfig{DSN: dsn}); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Execute(context.Background(), "CREATE TABLE widgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Execute(context.Background(), "CREATE TABLE gadgets (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}

	created := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "Target", "driver": "sqlite", "dsn": dsn}, tok), app.routes())
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create target connection: status=%d body=%s", created.StatusCode, created.BodyBytes)
	}
	connectionID := int64(created.BodyFields["id"].(float64))

	refreshRes := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connectionID, 10))+"/schema/refresh", nil, tok), app.routes())
	if refreshRes.StatusCode != http.StatusOK {
		t.Fatalf("schema refresh: status=%d body=%s", refreshRes.StatusCode, refreshRes.BodyBytes)
	}

	ctx := context.Background()
	_, directory, found, err := app.schemaSnapshots.Active(ctx, connectionID)
	if err != nil || !found {
		t.Fatalf("expected active snapshot: found=%v err=%v", found, err)
	}
	if !directory.Roots[0].Lazy {
		t.Fatalf("expected the scope with 2 objects over threshold 1 to be marked lazy: %+v", directory.Roots[0])
	}
	var ref metadata.ObjectRef
	for _, group := range directory.Roots[0].Groups {
		for _, candidate := range group.Objects {
			if candidate.Name == "widgets" {
				ref = candidate
			}
		}
	}
	if ref.Name != "widgets" {
		t.Fatalf("expected widgets ref in directory: %+v", directory.Roots[0])
	}

	endpoint := orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connectionID, 10))
	req := newAuthRequest(t, http.MethodPost, endpoint+"/schema/objects", map[string]any{"refs": []metadata.ObjectRef{ref}}, tok)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("objects without session: status=%d body=%s", res.StatusCode, res.BodyBytes)
	}
	objects, ok := res.BodyFields["objects"].([]any)
	if !ok || len(objects) != 0 {
		t.Fatalf("expected objects to be an empty array, got %+v (field type %T)", res.BodyFields["objects"], res.BodyFields["objects"])
	}
	pending, ok := res.BodyFields["pending_connection"].([]any)
	if !ok || len(pending) != 1 {
		t.Fatalf("expected the uncached ref to come back as pending_connection: %+v", res.BodyFields)
	}
	pendingRef, ok := pending[0].(map[string]any)
	if !ok || pendingRef["name"] != "widgets" {
		t.Fatalf("expected widgets in pending_connection: %+v", pending)
	}

	snapshot, _, found, err := app.schemaSnapshots.Active(ctx, connectionID)
	if err != nil || !found {
		t.Fatalf("expected active snapshot to remain: found=%v err=%v", found, err)
	}
	stored, err := app.schemaSnapshots.Objects(ctx, snapshot.ID, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("expected nothing written into the snapshot without a live fetch, got %d", len(stored))
	}
}
