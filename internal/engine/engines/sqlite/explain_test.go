package sqlite

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sqlwarden/internal/engine/explain"
)

func TestSQLiteExplain(t *testing.T) {
	driver := &sqliteDriver{}

	if driver.ExplainSpec().SupportsAnalyze {
		t.Fatal("expected sqlite not to support EXPLAIN ANALYZE")
	}

	plain, err := driver.Explain("SELECT 1", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	if want := (explain.Plan{Statement: "EXPLAIN QUERY PLAN SELECT 1"}); !reflect.DeepEqual(plain, want) {
		t.Fatalf("plain explain = %+v, want %+v", plain, want)
	}

	if _, err := driver.Explain("SELECT 1", explain.ModeAnalyze); !errors.Is(err, explain.ErrAnalyzeUnsupported) {
		t.Fatalf("expected ErrAnalyzeUnsupported, got %v", err)
	}
}

func TestSQLiteExplainRejectsMultipleStatements(t *testing.T) {
	driver := &sqliteDriver{}

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

func TestSQLiteExplainRejectsAlreadyExplainedStatement(t *testing.T) {
	driver := &sqliteDriver{}

	cases := []string{
		"EXPLAIN SELECT 1",
		"explain select 1",
		"EXPLAIN QUERY PLAN SELECT 1",
		"  -- leading comment\nEXPLAIN SELECT 1",
		"/* leading block comment */ EXPLAIN SELECT 1",
	}
	for _, sql := range cases {
		if _, err := driver.Explain(sql, explain.ModePlain); !errors.Is(err, explain.ErrAlreadyExplained) {
			t.Fatalf("Explain(%q, plain) err = %v, want ErrAlreadyExplained", sql, err)
		}
	}
}

func TestSQLiteExplainAllowsExplainAsIdentifierPrefix(t *testing.T) {
	driver := &sqliteDriver{}

	// "explaining" must not match the EXPLAIN keyword on a word boundary.
	if _, err := driver.Explain("SELECT * FROM explaining", explain.ModePlain); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLiteExplainRejectsAlreadyExplained(t *testing.T) {
	_, err := (&sqliteDriver{}).Explain("EXPLAIN QUERY PLAN SELECT 1", explain.ModePlain)
	if !errors.Is(err, explain.ErrAlreadyExplained) {
		t.Fatalf("err = %v, want ErrAlreadyExplained", err)
	}
}

func TestSQLiteExplainUnparseablePassesThrough(t *testing.T) {
	// Not this layer's job to reject; DB reports it.
	p, err := (&sqliteDriver{}).Explain("SELCT bogus", explain.ModePlain)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if p.Statement == "" {
		t.Fatalf("expected a plan statement")
	}
}
