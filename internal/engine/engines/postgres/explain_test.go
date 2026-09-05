package postgres

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sqlwarden/internal/engine/explain"
)

func TestPostgresExplain(t *testing.T) {
	driver := &Driver{}

	if !driver.ExplainSpec().SupportsAnalyze {
		t.Fatal("expected postgres to support EXPLAIN ANALYZE")
	}

	plain, err := driver.Explain("SELECT 1", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	if want := (explain.Plan{Statement: "EXPLAIN (FORMAT TEXT) SELECT 1"}); !reflect.DeepEqual(plain, want) {
		t.Fatalf("plain explain = %+v, want %+v", plain, want)
	}

	analyze, err := driver.Explain("SELECT 1", explain.ModeAnalyze)
	if err != nil {
		t.Fatal(err)
	}
	if want := (explain.Plan{Statement: "EXPLAIN (ANALYZE, FORMAT TEXT) SELECT 1"}); !reflect.DeepEqual(analyze, want) {
		t.Fatalf("analyze explain = %+v, want %+v", analyze, want)
	}
}

func TestPostgresExplainRejectsMultipleStatements(t *testing.T) {
	driver := &Driver{}

	cases := []string{
		"SELECT 1; SELECT 2",
		"",
		"   ",
	}
	for _, sql := range cases {
		if _, err := driver.Explain(sql, explain.ModePlain); !errors.Is(err, explain.ErrMultipleStatements) {
			t.Fatalf("Explain(%q, plain) err = %v, want ErrMultipleStatements", sql, err)
		}
	}
}

func TestPostgresExplainRejectsAlreadyExplainedStatement(t *testing.T) {
	driver := &Driver{}

	cases := []string{
		"EXPLAIN SELECT 1",
		"explain select 1",
		"  -- leading comment\nEXPLAIN SELECT 1",
		"EXPLAIN ANALYZE SELECT 1",
	}
	for _, sql := range cases {
		if _, err := driver.Explain(sql, explain.ModePlain); !errors.Is(err, explain.ErrAlreadyExplained) {
			t.Fatalf("Explain(%q, plain) err = %v, want ErrAlreadyExplained", sql, err)
		}
		if _, err := driver.Explain(sql, explain.ModeAnalyze); !errors.Is(err, explain.ErrAlreadyExplained) {
			t.Fatalf("Explain(%q, analyze) err = %v, want ErrAlreadyExplained", sql, err)
		}
	}
}
