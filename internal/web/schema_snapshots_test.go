package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/jobs"
	schemaapp "github.com/sqlwarden/internal/schema"
)

func TestSchemaSnapshotStorePublishesAndRetainsTwoGenerations(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-store"), "Snapshot Store", "Snapshot Store Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Snapshot Conn", "open")

	var last schemaapp.Snapshot
	for generation := 1; generation <= 3; generation++ {
		objectName := fmt.Sprintf("widgets_%d", generation)
		directory := snapshotDirectory(objectName, time.Now().Add(time.Duration(generation)*time.Second))
		snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, directory)
		if err != nil {
			t.Fatal(err)
		}
		object := metadata.Object{
			Ref: metadata.ObjectRef{Scope: directory.DefaultScope, Kind: "table", Name: objectName},
			Relational: &metadata.RelationalDetail{
				Columns: []metadata.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}},
			},
		}
		if err := app.schemaSnapshots.PutObjects(context.Background(), snapshot.ID, []metadata.Object{object}); err != nil {
			t.Fatal(err)
		}
		if err := app.schemaSnapshots.PutRelationship(context.Background(), snapshot.ID, &metadata.RelationshipGraph{
			Scope: directory.DefaultScope,
		}); err != nil {
			t.Fatal(err)
		}
		if err := app.schemaSnapshots.Publish(context.Background(), snapshot.ID); err != nil {
			t.Fatal(err)
		}
		last = snapshot
	}

	active, directory, found, err := app.schemaSnapshots.Active(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, true)
	assert.Equal(t, active.ID, last.ID)
	assert.Equal(t, directory.Roots[0].Groups[0].Objects[0].Name, "widgets_3")

	if err := app.schemaSnapshots.Publish(context.Background(), last.ID); err == nil {
		t.Fatal("expected an already-ready snapshot to reject publication")
	}
	activeAfterRetry, _, found, err := app.schemaSnapshots.Active(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, true)
	assert.Equal(t, activeAfterRetry.ID, last.ID)

	objects, err := app.schemaSnapshots.Objects(context.Background(), active.ID, []metadata.ObjectRef{
		{Scope: directory.DefaultScope, Kind: "table", Name: "widgets_3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(objects), 1)
	assert.Equal(t, objects[0].Ref.Name, "widgets_3")
	allObjects, err := app.schemaSnapshots.AllObjects(context.Background(), active.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(allObjects), 1)
	assert.Equal(t, allObjects[0].Ref.Name, "widgets_3")

	graph, found, err := app.schemaSnapshots.Relationship(context.Background(), active.ID, directory.DefaultScope)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, true)
	assert.Equal(t, graph.Scope, directory.DefaultScope)

	count, err := app.db.NewSelect().Model((*schemaapp.Snapshot)(nil)).
		Where("connection_id = ?", conn.ID).
		Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, 2)
}

func TestSchemaSnapshotStoreRejectsSupersededGeneration(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-order"), "Snapshot Order", "Snapshot Order Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Snapshot Conn", "open")

	older, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, snapshotDirectory("older", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	newer, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, snapshotDirectory("newer", time.Now().Add(time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(context.Background(), newer.ID); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, errors.Is(app.schemaSnapshots.Publish(context.Background(), older.ID), schemaapp.ErrSnapshotSuperseded), true)
	if err := app.schemaSnapshots.Abort(context.Background(), older.ID); err != nil {
		t.Fatal(err)
	}

	active, directory, found, err := app.schemaSnapshots.Active(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, true)
	assert.Equal(t, active.ID, newer.ID)
	assert.Equal(t, directory.Roots[0].Groups[0].Objects[0].Name, "newer")
}

func TestManualPersistentSchemaRefreshRunsSynchronously(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-manual"), "Snapshot Manual", "Snapshot Manual Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)

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
	if err := driver.Close(); err != nil {
		t.Fatal(err)
	}

	created := send(t, newAuthRequest(t, http.MethodPost, orgEnvConnectionsURL(org.Slug, ws.ID, envID),
		map[string]any{"name": "Target", "driver": "sqlite", "dsn": dsn}, tok), app.routes())
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create target connection: status=%d body=%s", created.StatusCode, created.BodyBytes)
	}
	connectionID := int64(created.BodyFields["id"].(float64))

	res := send(t, newAuthRequest(t, http.MethodPost,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(connectionID, 10))+"/schema/refresh", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["status"], "ok")
	assert.Equal(t, res.BodyFields["mode"], "persistent")
	if res.BodyFields["snapshot_id"] == "" || res.BodyFields["generated_at"] == "" {
		t.Fatalf("expected completed snapshot metadata, got %v", res.BodyFields)
	}

	_, directory, found, err := app.schemaSnapshots.Active(context.Background(), connectionID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, true)
	refs := directoryObjectRefs(directory)
	if len(refs) != 1 || refs[0].Name != "widgets" {
		t.Fatalf("expected refreshed widgets table, got %v", refs)
	}
	_, activeJob, err := app.workspaceJobStore().ActiveBySingletonKey(context.Background(), schemaSyncSingletonKey(connectionID))
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, activeJob, false)
	jobCount, err := app.db.NewSelect().Model((*jobs.Record)(nil)).
		Where("type = ? AND singleton_key = ?", jobs.TypeSchemaSync, schemaSyncSingletonKey(connectionID)).
		Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, jobCount, 0)
}

func TestSchemaSnapshotPublishRechecksPolicy(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-policy"), "Snapshot Policy", "Snapshot Policy Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Snapshot Conn", "open")

	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, snapshotDirectory("widgets", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.db.UpdateConnectionWithPolicy(context.Background(), conn.ID, conn.Name, conn.DSNEncrypted, conn.AccessMode, database.SchemaSnapshotPolicyDisabled); err != nil {
		t.Fatal(err)
	}
	err = app.schemaSnapshots.Publish(context.Background(), snapshot.ID)
	assert.Equal(t, errors.Is(err, schemaapp.ErrSnapshotsDisabled), true)

	_, _, found, err := app.schemaSnapshots.Active(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, false)
}

func TestPersistentSchemaDirectoryDoesNotRequireSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-http"), "Snapshot HTTP", "Snapshot HTTP Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Snapshot Conn", "open")

	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, snapshotDirectory("widgets", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(context.Background(), snapshot.ID); err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodGet,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10))+"/schema/directory", nil, tok)
	res := send(t, req, app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)

	directory := res.BodyFields["directory"].(map[string]any)
	roots := directory["roots"].([]any)
	groups := roots[0].(map[string]any)["groups"].([]any)
	objects := groups[0].(map[string]any)["objects"].([]any)
	assert.Equal(t, objects[0].(map[string]any)["name"], "widgets")
}

func TestDisablingConnectionSnapshotsPurgesStoredMetadata(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-disable"), "Snapshot Disable", "Snapshot Disable Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Snapshot Conn", "open")
	encryptedDSN, err := app.keyring.Encrypt(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.db.UpdateConnectionWithPolicy(context.Background(), conn.ID, conn.Name, encryptedDSN, conn.AccessMode, database.SchemaSnapshotPolicyInherit); err != nil {
		t.Fatal(err)
	}

	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, snapshotDirectory("widgets", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(context.Background(), snapshot.ID); err != nil {
		t.Fatal(err)
	}

	req := newAuthRequest(t, http.MethodPatch,
		orgConnectionURL(org.Slug, ws.ID, envID, strconv.FormatInt(conn.ID, 10)),
		map[string]any{"schema_snapshot_policy": database.SchemaSnapshotPolicyDisabled}, tok)
	res := send(t, req, app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("disable snapshots: status=%d body=%s", res.StatusCode, string(res.BodyBytes))
	}

	_, _, found, err := app.schemaSnapshots.Active(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, false)

	enabled, err := app.db.SchemaSnapshotsEnabled(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, enabled, false)
}

func TestDisablingOrganizationSnapshotsPurgesStoredMetadata(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, tok, org := seedOrgOwner(t, app, uniqueEmail(t, "snapshot-org-disable"), "Snapshot Org Disable", "Snapshot Org Disable")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Snapshot WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Snapshot Conn", "open")

	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID, snapshotDirectory("widgets", time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := app.schemaSnapshots.Publish(context.Background(), snapshot.ID); err != nil {
		t.Fatal(err)
	}

	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug,
		map[string]any{"schema_snapshots_enabled": false}, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	assert.Equal(t, res.BodyFields["schema_snapshots_enabled"], false)

	_, _, found, err := app.schemaSnapshots.Active(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, found, false)

	enabled, err := app.db.SchemaSnapshotsEnabled(context.Background(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, enabled, false)
}

type fakeBatchInspector struct {
	failOnCall int
	calls      int
	batchSizes []int
}

func (f *fakeBatchInspector) SchemaSpec() metadata.SchemaSpec { return metadata.SchemaSpec{} }

func (f *fakeBatchInspector) InspectDirectory(context.Context, metadata.DirectoryOptions) (*metadata.Directory, error) {
	return &metadata.Directory{}, nil
}

func (f *fakeBatchInspector) InspectObjects(_ context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	f.calls++
	f.batchSizes = append(f.batchSizes, len(refs))
	if f.failOnCall != 0 && f.calls == f.failOnCall {
		return nil, errors.New("inspect failed")
	}
	out := make([]metadata.Object, 0, len(refs))
	for _, ref := range refs {
		out = append(out, metadata.Object{
			Ref:        ref,
			Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}}},
		})
	}
	return out, nil
}

func batchInspectorRefs(n int) (metadata.ScopePath, []metadata.ObjectRef) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	refs := make([]metadata.ObjectRef, n)
	for i := range refs {
		refs[i] = metadata.ObjectRef{Scope: scope, Kind: "table", Name: fmt.Sprintf("t_%d", i)}
	}
	return scope, refs
}

func TestInspectAndStoreObjectsPersistsEveryBatch(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "batch-store"), "Batch Store", "Batch Store Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Batch WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Batch Conn", "open")

	total := schemaObjectBatchSize*2 + 7
	scope, refs := batchInspectorRefs(total)
	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID,
		&metadata.Directory{Engine: "sqlite", DefaultScope: scope, GeneratedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	inspector := &fakeBatchInspector{}
	count, err := app.inspectAndStoreObjects(context.Background(), inspector, snapshot.ID, refs)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, count, total)
	assert.Equal(t, inspector.calls, 3)
	assert.Equal(t, inspector.batchSizes[0], schemaObjectBatchSize)
	assert.Equal(t, inspector.batchSizes[2], 7)

	all, err := app.schemaSnapshots.AllObjects(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, len(all), total)
}

func TestInspectAndStoreObjectsAbortsOnMidLoopInspectError(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	owner, _, org := seedOrgOwner(t, app, uniqueEmail(t, "batch-err"), "Batch Err", "Batch Err Org")
	ws := seedWorkspaceForAccount(t, app, org, owner, "Batch WS", "")
	envID := defaultEnvironmentID(t, app, ws.ID)
	conn := seedConnection(t, app, ws.ID, &envID, org.ID, "sqlite", "Batch Conn", "open")

	total := schemaObjectBatchSize*3 + 1
	scope, refs := batchInspectorRefs(total)
	snapshot, err := app.schemaSnapshots.Begin(context.Background(), conn.ID, &org.ID,
		&metadata.Directory{Engine: "sqlite", DefaultScope: scope, GeneratedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}

	_, err = app.inspectAndStoreObjects(context.Background(), &fakeBatchInspector{failOnCall: 2}, snapshot.ID, refs)
	var coded jobs.CodedError
	if !errors.As(err, &coded) || coded.Code != "schema_objects_failed" || !coded.Retryable {
		t.Fatalf("expected retryable schema_objects_failed coded error, got %v", err)
	}

	all, err := app.schemaSnapshots.AllObjects(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) > schemaObjectBatchSize {
		t.Fatalf("expected at most one batch persisted before the failure, got %d", len(all))
	}
}

func snapshotDirectory(objectName string, generatedAt time.Time) *metadata.Directory {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	return &metadata.Directory{
		Engine: "sqlite", DefaultScope: scope, GeneratedAt: generatedAt,
		Roots: []metadata.ScopeNode{{
			Path: scope,
			Groups: []metadata.ObjectGroup{{
				Kind: "table",
				Objects: []metadata.ObjectRef{{
					Scope: scope,
					Kind:  "table",
					Name:  objectName,
				}},
			}},
		}},
	}
}
