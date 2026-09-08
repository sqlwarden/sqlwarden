package sqlserver

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/completer"
)

func TestSQLServerCompleteKeyword(t *testing.T) {
	driver := &Driver{}
	result, err := driver.Complete(context.Background(), completer.Request{
		SQL: "SELE", CursorOffset: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.Label == "SELECT" && suggestion.Kind == "keyword" {
			return
		}
	}
	t.Fatalf("expected SELECT keyword suggestion, got %+v", result.Suggestions)
}
