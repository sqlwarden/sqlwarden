package web

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var rawConnSeq atomic.Int64

func seedRawConnection(t *testing.T, app *application, driver, dsn string) int64 {
	t.Helper()
	ctx := context.Background()

	n := rawConnSeq.Add(1)
	org, err := app.db.InsertOrg(ctx, fmt.Sprintf("org-%d", n), "Org")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := app.db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.db.InsertEnvironment(ctx, ws.ID, fmt.Sprintf("Env-%d", n), "")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := app.keyring.Encrypt(dsn)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := app.db.InsertConnection(ctx, ws.ID, &env.ID, "Conn", driver, enc, "open")
	if err != nil {
		t.Fatal(err)
	}
	return conn.ID
}

func TestBackfillConnectionTLSConfig(t *testing.T) {
	app := newTestApplication(t)
	ctx := context.Background()

	pgID := seedRawConnection(t, app, "postgres", "postgres://u:p@h:5432/db?sslmode=verify-full&application_name=x")
	preferID := seedRawConnection(t, app, "postgres", "postgres://u:p@h:5432/db?sslmode=prefer")
	oraID := seedRawConnection(t, app, "oracle", "oracle://u:p@h:1521/ORCLPDB1?SSL=true")
	plainID := seedRawConnection(t, app, "sqlite", "file:/tmp/x.db")

	rep, err := app.backfillConnectionTLSConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Migrated != 3 {
		t.Fatalf("migrated=%d want 3", rep.Migrated)
	}

	assertMode := func(id int64, want string) {
		c, _, _ := app.db.GetConnection(ctx, id)
		doc, has, err := app.decodeTLSDocument(c.TLSConfigEncrypted)
		if err != nil || !has || doc.Mode != want {
			t.Fatalf("conn %d: mode=%q has=%v err=%v want %q", id, doc.Mode, has, err, want)
		}
		dsn, _ := app.keyring.Decrypt(c.DSNEncrypted)
		if strings.Contains(dsn, "sslmode=") || strings.Contains(dsn, "SSL=") {
			t.Fatalf("conn %d: verification param not stripped from DSN: %s", id, dsn)
		}
	}
	assertMode(pgID, "verify-full")
	assertMode(preferID, "require")
	assertMode(oraID, "require")

	plain, _, _ := app.db.GetConnection(ctx, plainID)
	if plain.TLSConfigEncrypted != "" {
		t.Fatal("sqlite connection should be untouched")
	}

	rep2, err := app.backfillConnectionTLSConfig(ctx)
	if err != nil || rep2.Migrated != 0 {
		t.Fatalf("second run migrated=%d err=%v want 0", rep2.Migrated, err)
	}
}
