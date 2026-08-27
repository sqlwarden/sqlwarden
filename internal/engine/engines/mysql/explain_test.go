package mysql

import (
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/explain"
)

func TestMySQLExplain(t *testing.T) {
	driver := &mysqlDriver{}

	if !driver.ExplainSpec().SupportsAnalyze {
		t.Fatal("expected mysql to support EXPLAIN ANALYZE")
	}

	plain, err := driver.Explain("SELECT 1", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	if want := "EXPLAIN SELECT 1"; plain != want {
		t.Fatalf("plain explain = %q, want %q", plain, want)
	}

	analyze, err := driver.Explain("SELECT 1", explain.ModeAnalyze)
	if err != nil {
		t.Fatal(err)
	}
	if want := "EXPLAIN ANALYZE SELECT 1"; analyze != want {
		t.Fatalf("analyze explain = %q, want %q", analyze, want)
	}
}

func TestMySQLExplainRejectsMultipleStatements(t *testing.T) {
	driver := &mysqlDriver{}

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

func TestMySQLExplainRejectsAlreadyExplainedStatement(t *testing.T) {
	driver := &mysqlDriver{}

	cases := []string{
		"EXPLAIN SELECT 1",
		"explain select 1",
		"DESCRIBE mytable",
		"  -- leading comment\nEXPLAIN SELECT 1",
	}
	for _, sql := range cases {
		if _, err := driver.Explain(sql, explain.ModePlain); !errors.Is(err, explain.ErrAlreadyExplained) {
			t.Fatalf("Explain(%q, plain) err = %v, want ErrAlreadyExplained", sql, err)
		}
	}
}
