package oracle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sqlwarden/internal/engine/explain"
)

func TestOracleExplain(t *testing.T) {
	driver := &oracleDriver{}

	if !driver.ExplainSpec().SupportsAnalyze {
		t.Fatal("expected oracle to support EXPLAIN ANALYZE")
	}

	plain, err := driver.Explain("SELECT 1 FROM dual", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	wantPlain := explain.Plan{
		Setup:     []string{"EXPLAIN PLAN FOR SELECT 1 FROM dual"},
		Statement: "SELECT plan_table_output FROM TABLE(DBMS_XPLAN.DISPLAY(NULL, NULL, 'TYPICAL'))",
	}
	if !reflect.DeepEqual(plain, wantPlain) {
		t.Fatalf("plain explain = %+v, want %+v", plain, wantPlain)
	}

	analyze, err := driver.Explain("SELECT 1 FROM dual", explain.ModeAnalyze)
	if err != nil {
		t.Fatal(err)
	}
	wantAnalyze := explain.Plan{
		Setup: []string{
			"ALTER SESSION SET STATISTICS_LEVEL = ALL",
			"SELECT 1 FROM dual",
		},
		Statement: "SELECT plan_table_output FROM TABLE(DBMS_XPLAN.DISPLAY_CURSOR(NULL, NULL, 'ALLSTATS LAST'))",
		Teardown:  []string{"ALTER SESSION SET STATISTICS_LEVEL = TYPICAL"},
	}
	if !reflect.DeepEqual(analyze, wantAnalyze) {
		t.Fatalf("analyze explain = %+v, want %+v", analyze, wantAnalyze)
	}
}

func TestOracleExplainTrimsTrailingSemicolon(t *testing.T) {
	driver := &oracleDriver{}

	plan, err := driver.Explain("SELECT 1 FROM dual ;  ", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plan.Setup[0], "EXPLAIN PLAN FOR SELECT 1 FROM dual"; got != want {
		t.Fatalf("setup = %q, want %q", got, want)
	}
}

func TestOracleExplainRejectsMultipleStatements(t *testing.T) {
	driver := &oracleDriver{}

	cases := []string{
		"SELECT 1 FROM dual; SELECT 2 FROM dual",
		"",
		"   ",
	}
	for _, sql := range cases {
		if _, err := driver.Explain(sql, explain.ModePlain); !errors.Is(err, explain.ErrMultipleStatements) {
			t.Fatalf("Explain(%q, plain) err = %v, want ErrMultipleStatements", sql, err)
		}
	}
}

func TestOracleExplainRejectsAlreadyExplainedStatement(t *testing.T) {
	driver := &oracleDriver{}

	cases := []string{
		"EXPLAIN PLAN FOR SELECT 1 FROM dual",
		"explain plan for select 1 from dual",
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

func TestOracleExplainRejectsUnknownMode(t *testing.T) {
	driver := &oracleDriver{}

	if _, err := driver.Explain("SELECT 1 FROM dual", explain.Mode("bogus")); !errors.Is(err, explain.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
}
