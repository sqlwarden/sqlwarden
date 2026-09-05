package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/completer"
)

func connectedSQLite(t *testing.T, ddl ...string) *sqliteDriver {
	t.Helper()
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	for _, s := range ddl {
		if _, err := d.Execute(context.Background(), s); err != nil {
			t.Fatalf("Execute %q: %v", s, err)
		}
	}
	return d
}

func TestSQLiteCompleteKeywordsNoSchema(t *testing.T) {
	d := &sqliteDriver{}
	res, err := d.Complete(context.Background(), completer.Request{SQL: "SEL", CursorOffset: 3})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	found := false
	for _, s := range res.Suggestions {
		if strings.EqualFold(s.Label, "SELECT") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SELECT among %d suggestions", len(res.Suggestions))
	}
}

func TestSQLiteCompletionVocabularyStable(t *testing.T) {
	d := &sqliteDriver{}
	a := d.CompletionVocabulary()
	b := d.CompletionVocabulary()
	if a.Dialect != "sqlite" || a.Version == "" {
		t.Fatalf("bad vocabulary header: %+v", a)
	}
	if a.Version != b.Version || len(a.Suggestions) != len(b.Suggestions) {
		t.Fatalf("vocabulary not deterministic")
	}
	if len(a.Suggestions) == 0 {
		t.Fatalf("empty vocabulary")
	}
}

func TestSQLiteCompleteCursorOutOfRange(t *testing.T) {
	d := &sqliteDriver{}
	if _, err := d.Complete(context.Background(), completer.Request{SQL: "SELECT", CursorOffset: 99}); err == nil {
		t.Fatalf("expected out-of-range error")
	}
}

func TestSQLiteInvalidateCompletionCatalogNoPanic(t *testing.T) {
	d := &sqliteDriver{}
	d.InvalidateCompletionCatalog("conn-1") // must be safe with nothing cached
}
