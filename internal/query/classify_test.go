package query

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/classifier"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
)

func TestEngineClassifierFallsBackToHeuristic(t *testing.T) {
	// Every registered engine ships a real classifier, so the heuristic
	// fallback is only reachable for a driver name that resolves to no engine.
	if _, ok := RegisteredClassifier("driver-with-no-engine"); ok {
		t.Fatal("an unregistered driver must not advertise a registered classifier")
	}

	c := EngineAnalyzers{}.Classifier("driver-with-no-engine")
	tests := []struct {
		sql  string
		want classifier.Kind
	}{
		{sql: "SELECT 1", want: classifier.KindDQL},
		{sql: "UPDATE widgets SET active = false", want: classifier.KindDML},
		{sql: "DROP TABLE widgets", want: classifier.KindDDL},
		{sql: "VACUUM", want: classifier.KindUnknown},
	}
	for _, test := range tests {
		got, err := c.Classify(context.Background(), classifier.Request{SQL: test.sql})
		if err != nil {
			t.Fatalf("Classify(%q): %v", test.sql, err)
		}
		if got.Kind != test.want || got.Source != "heuristic" {
			t.Errorf("Classify(%q) = %+v, want kind=%q source=heuristic", test.sql, got, test.want)
		}
	}
}

func TestRegisteredClassifierUsesSQLiteEngine(t *testing.T) {
	c, ok := RegisteredClassifier("sqlite")
	if !ok {
		t.Fatal("sqlite must advertise a registered classifier")
	}

	tests := []struct {
		sql  string
		want classifier.Kind
	}{
		{sql: "SELECT 1", want: classifier.KindDQL},
		{sql: "DELETE FROM t", want: classifier.KindDML},
		{sql: "PRAGMA foreign_keys", want: classifier.KindUnknown},
	}
	for _, test := range tests {
		got, err := c.Classify(context.Background(), classifier.Request{SQL: test.sql})
		if err != nil {
			t.Fatalf("Classify(%q): %v", test.sql, err)
		}
		if got.Kind != test.want {
			t.Errorf("Classify(%q) kind = %q, want %q", test.sql, got.Kind, test.want)
		}
	}
}

func TestEngineExplainerUnknownDriverReturnsNotOK(t *testing.T) {
	if _, ok := (EngineAnalyzers{}).Explainer("not-a-real-driver"); ok {
		t.Fatal("expected an unregistered driver to report no Explainer")
	}
}

func TestEngineSafetyCheckerFallsBackToHeuristic(t *testing.T) {
	if (EngineAnalyzers{}).SafetyChecker("driver-with-no-engine") == nil {
		t.Fatal("an unregistered driver must fall back to the heuristic checker")
	}
}
