package sqlite

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func TestTargetKindDistinguishesInMemoryFromFile(t *testing.T) {
	driver, err := engine.New("sqlite")
	if err != nil {
		t.Fatal(err)
	}
	classifier, ok := driver.(engine.TargetClassifier)
	if !ok {
		t.Fatal("sqlite driver does not implement engine.TargetClassifier")
	}
	cases := map[string]engine.TargetKind{
		":memory:":                   engine.TargetKindInMemory,
		" :memory: ":                 engine.TargetKindInMemory,
		"file::memory:?cache=shared": engine.TargetKindInMemory,
		"/tmp/data.db":               engine.TargetKindLocal,
		"file:/tmp/data.db?mode=ro":  engine.TargetKindLocal,
		"file:memory.db":             engine.TargetKindLocal,
	}
	for dsn, want := range cases {
		if got := classifier.TargetKind(dsn); got != want {
			t.Errorf("TargetKind(%q) = %q, want %q", dsn, got, want)
		}
	}
}
