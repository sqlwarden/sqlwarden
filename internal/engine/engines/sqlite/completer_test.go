package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/completer"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
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

func sqliteCompletionView(objects ...metadata.Object) *metadata.CompletionView {
	main := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	return metadatatest.Build((&sqliteDriver{}).Tree(), metadatatest.Fixture{DefaultScope: main, Objects: objects})
}

func TestSQLiteCompleteReturnsDemandsForUnlistedTables(t *testing.T) {
	tree := (&sqliteDriver{}).Tree()
	main := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	view := metadata.NewCompletionView(tree, main, "", nil, nil)
	result, err := (&sqliteDriver{}).Complete(context.Background(), completer.Request{SQL: "SELECT * FROM ", CursorOffset: 14, Metadata: view})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Demands) == 0 || result.Demands[0].Parent != main {
		t.Fatalf("demands = %v", result.Demands)
	}
}
