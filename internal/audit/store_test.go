package audit_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/audit/audittest"
	"github.com/sqlwarden/internal/database"
)

func TestSQLStoreContract(t *testing.T) {
	audittest.Run(t, func(t *testing.T) audittest.Subject {
		store := audit.NewSQLStore(newAuditDB(t).DB)
		return audittest.Subject{
			Writer: audit.NewCoreWriter(store, time.Now),
			Recorded: func(ctx context.Context) ([]audit.Event, error) {
				return store.Events(ctx, 0)
			},
		}
	})
}

func newAuditDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New("sqlite", filepath.Join(t.TempDir(), "audit.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}
	return db
}
