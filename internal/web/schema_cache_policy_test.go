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
	if err := app.db.UpsertSchemaListings(context.Background(), []database.SchemaListing{{
		ConnectionID: connID, ParentPath: "", Folder: "databases", ChildrenData: []byte("stale"), FetchedAt: time.Now().UTC(),
	}}); err != nil {
		t.Fatal(err)
	}
}

func cachedListingCount(t *testing.T, app *application, connID int64) int {
	t.Helper()
	rows, err := app.db.SchemaListingsWithin(context.Background(), connID, "")
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestDisablingConnectionSnapshotsPurgesNodeCache(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	encryptedDSN, err := f.app.keyring.Encrypt("dsn")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.app.db.UpdateConnectionDSN(context.Background(), f.conn.ID, encryptedDSN); err != nil {
		t.Fatal(err)
	}
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
	if err := f.app.disableOrganizationSnapshots(context.Background(), f.orgID); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, cachedListingCount(t, f.app, f.conn.ID), 0)
}

func TestRotatingConnectionDSNPurgesNodeCache(t *testing.T) {
	t.Parallel()
	f := newNavFixture(t, navTestEngine)
	encryptedDSN, err := f.app.keyring.Encrypt("dsn")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.app.db.UpdateConnectionDSN(context.Background(), f.conn.ID, encryptedDSN); err != nil {
		t.Fatal(err)
	}
	seedCachedListing(t, f.app, f.conn.ID)

	res := send(t, newAuthRequest(t, http.MethodPatch, f.base,
		map[string]any{"dsn": "other-dsn"}, f.tok), f.app.routes())
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("rotate dsn: status=%d body=%s", res.StatusCode, res.BodyBytes)
	}
	assert.Equal(t, cachedListingCount(t, f.app, f.conn.ID), 0)
}
