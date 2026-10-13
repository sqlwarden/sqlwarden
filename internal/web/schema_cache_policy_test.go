package web

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
)

func seedCachedListing(t *testing.T, app *application, connID int64) {
	t.Helper()
	if err := testDB(app).UpsertSchemaListings(context.Background(), []database.SchemaListing{{
		ConnectionID: connID, ParentPath: "", Folder: "databases", ChildrenData: []byte("stale"), FetchedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
}

func cachedListingCount(t *testing.T, app *application, connID int64) int {
	t.Helper()
	rows, err := testDB(app).SchemaListingsWithin(context.Background(), connID, "")
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestDisablingConnectionSnapshotsPurgesNodeCache(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	encryptedDSN, err := testKeyring(f.app).Encrypt("dsn")
	if err != nil {
		t.Fatal(err)
	}
	setLegacyColumn(t, f.app, f.conn.ID, "dsn_encrypted", encryptedDSN)
	seedCachedListing(t, f.app, f.conn.ID)

	res := send(t, newAuthRequest(t, http.MethodPatch, f.base,
		map[string]any{"schema_snapshot_policy": database.SchemaSnapshotPolicyDisabled}, f.tok), f.app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("disable snapshots: status=%d body=%s", res.StatusCode, res.BodyBytes)
	}
	assert.Equal(t, cachedListingCount(t, f.app, f.conn.ID), 0)
}

func TestDisablingOrganizationSnapshotsPurgesNodeCache(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	seedCachedListing(t, f.app, f.conn.ID)
	org, found, err := testDB(f.app).GetOrg(context.Background(), f.orgID)
	if err != nil || !found {
		t.Fatalf("load org: found=%v err=%v", found, err)
	}
	res := send(t, newAuthRequest(t, http.MethodPatch, "/api/v1/orgs/"+org.Slug,
		map[string]any{"schema_snapshots_enabled": false}, f.tok), f.app.routes())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("disable org snapshots: status=%d body=%s", res.StatusCode, res.BodyBytes)
	}
	assert.Equal(t, cachedListingCount(t, f.app, f.conn.ID), 0)
}

func TestRotatingConnectionDSNPurgesNodeCache(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	encryptedDSN, err := testKeyring(f.app).Encrypt("dsn")
	if err != nil {
		t.Fatal(err)
	}
	setLegacyColumn(t, f.app, f.conn.ID, "dsn_encrypted", encryptedDSN)
	seedCachedListing(t, f.app, f.conn.ID)

	res := send(t, newAuthRequest(t, http.MethodPatch, f.base,
		map[string]any{"params": map[string]any{"dsn": "other-dsn"}}, f.tok), f.app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("rotate dsn: status=%d body=%s", res.StatusCode, res.BodyBytes)
	}
	assert.Equal(t, cachedListingCount(t, f.app, f.conn.ID), 0)
}
