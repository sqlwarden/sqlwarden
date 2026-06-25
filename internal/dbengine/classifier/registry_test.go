package classifier

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/driver"
)

func TestForFallsBackToHeuristic(t *testing.T) {
	c := For(driver.Dialect("nonesuch"))
	if c == nil {
		t.Fatal("For must never return nil for classifier")
	}
	got, _ := c.Classify(context.Background(), Request{SQL: "DROP TABLE t"})
	if got.Kind != KindDDL {
		t.Fatalf("heuristic kind = %q, want ddl", got.Kind)
	}
}

func TestRegisterAndFor(t *testing.T) {
	stub := stubClassifier{kind: KindDQL}
	Register(driver.DialectPostgres, stub)
	t.Cleanup(func() { unregister(driver.DialectPostgres) })

	got, _ := For(driver.DialectPostgres).Classify(context.Background(), Request{SQL: "x"})
	if got.Kind != KindDQL {
		t.Fatalf("registered classifier not returned: %+v", got)
	}
}

type stubClassifier struct{ kind Kind }

func (s stubClassifier) Classify(context.Context, Request) (Result, error) {
	return Result{Kind: s.kind, Source: "stub"}, nil
}
