package audit_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/audit/audittest"
	"github.com/sqlwarden/internal/database"
)

func TestCoreWriterContract(t *testing.T) {
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

func TestCoreWriterReportsStorageFailure(t *testing.T) {
	failure := errors.New("storage unavailable")
	writer := audit.NewCoreWriter(failingStore{err: failure}, time.Now)
	err := writer.Write(context.Background(), audit.Event{Action: "identity.account.register", Outcome: audit.OutcomeSuccess})
	if !errors.Is(err, failure) {
		t.Fatalf("write error = %v, want %v", err, failure)
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	clock := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	first := audit.Normalize(audit.Event{Action: "a", Outcome: audit.OutcomeSuccess}, func() time.Time { return clock })
	second := audit.Normalize(first, func() time.Time { return clock.Add(time.Hour) })
	if first.ID != second.ID || !first.OccurredAt.Equal(second.OccurredAt) {
		t.Fatalf("normalize reassigned identity: %+v then %+v", first, second)
	}
}

func TestEventsAfterUsesInsertionOrderNotEventIdentity(t *testing.T) {
	store := audit.NewSQLStore(newAuditDB(t).DB)
	writer := audit.NewCoreWriter(store, time.Now)
	ctx := context.Background()

	for _, event := range []audit.Event{
		{ID: "z-later-in-sort", Action: "first", Outcome: audit.OutcomeSuccess},
		{ID: "a-earlier-in-sort", Action: "second", Outcome: audit.OutcomeSuccess},
	} {
		if err := writer.Write(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	events, err := store.EventsAfter(ctx, "z-later-in-sort", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != "a-earlier-in-sort" {
		t.Fatalf("events after first insert = %+v, want the later insert regardless of id order", events)
	}
}

type failingStore struct{ err error }

func (s failingStore) InsertEvent(context.Context, audit.Event) error { return s.err }

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
