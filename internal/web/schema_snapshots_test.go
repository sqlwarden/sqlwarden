package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	schemameta "github.com/sqlwarden/internal/dbengine/schema"
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
		object := schemameta.Object{
			Ref: schemameta.ObjectRef{Scope: directory.DefaultScope, Kind: "table", Name: objectName},
			Relational: &schemameta.RelationalDetail{
				Columns: []schemameta.Column{{Name: "id", DataType: "INTEGER", Ordinal: 1}},
			},
		}
		if err := app.schemaSnapshots.PutObjects(context.Background(), snapshot.ID, []schemameta.Object{object}); err != nil {
			t.Fatal(err)
		}
		if err := app.schemaSnapshots.PutRelationship(context.Background(), snapshot.ID, &schemameta.RelationshipGraph{
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

	objects, err := app.schemaSnapshots.Objects(context.Background(), active.ID, []schemameta.ObjectRef{
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

func snapshotDirectory(objectName string, generatedAt time.Time) *schemameta.Directory {
	scope := schemameta.NewScopePath(schemameta.ScopeSegment{Kind: "database", Name: "main"})
	return &schemameta.Directory{
		Engine: "sqlite", DefaultScope: scope, GeneratedAt: generatedAt,
		Roots: []schemameta.ScopeNode{{
			Path: scope,
			Groups: []schemameta.ObjectGroup{{
				Kind: "table",
				Objects: []schemameta.ObjectRef{{
					Scope: scope,
					Kind:  "table",
					Name:  objectName,
				}},
			}},
		}},
	}
}
