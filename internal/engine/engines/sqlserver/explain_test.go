package sqlserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/explain"
)

func TestExplainPlain(t *testing.T) {
	d := &Driver{}
	plan, err := d.Explain("SELECT 1", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Statement == "" {
		t.Fatalf("want a non-empty statement, got %+v", plan)
	}
}

func TestExplainSpecDoesNotSupportAnalyze(t *testing.T) {
	d := &Driver{}
	if d.ExplainSpec().SupportsAnalyze {
		t.Fatal("want SupportsAnalyze false, got true")
	}
	if _, err := d.Explain("SELECT 1", explain.ModeAnalyze); !errors.Is(err, explain.ErrAnalyzeUnsupported) {
		t.Fatalf("want ErrAnalyzeUnsupported, got %v", err)
	}
}

func TestExplainRejectsMultipleStatements(t *testing.T) {
	d := &Driver{}
	if _, err := d.Explain("SELECT 1; SELECT 2;", explain.ModePlain); !errors.Is(err, explain.ErrMultipleStatements) {
		t.Fatalf("want ErrMultipleStatements, got %v", err)
	}
}

func TestExplainSetsUpAndTearsDownShowplan(t *testing.T) {
	d := &Driver{}
	plan, err := d.Explain("SELECT 1", explain.ModePlain)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Setup) != 1 || plan.Setup[0] != "SET SHOWPLAN_XML ON" {
		t.Fatalf("want SHOWPLAN_XML setup, got %+v", plan.Setup)
	}
	if plan.Statement != "SELECT 1" {
		t.Fatalf("want unmodified statement, got %q", plan.Statement)
	}
	if len(plan.Teardown) != 1 || plan.Teardown[0] != "SET SHOWPLAN_XML OFF" {
		t.Fatalf("want SHOWPLAN_XML teardown, got %+v", plan.Teardown)
	}
}

// TestExplainPlainEndToEndViaSession runs the plan produced by Explain
// through connection.Session.ExecuteExplainPlan — the same path the HTTP
// handler uses — against a real SQL Server instance. SET SHOWPLAN_XML ON is
// connection-scoped: without ExecuteExplainPlan pinning Setup/Statement/
// Teardown to one physical connection, the driver's *sql.DB pool can (and,
// against a real server, reliably does) hand the Statement a different
// pooled connection than Setup ran on, so the query executes normally
// instead of returning plan XML.
func TestExplainPlainEndToEndViaSession(t *testing.T) {
	d := newConnectedDriver(t)
	s := &connection.Session{ID: "s1", AccountID: "a1", ConnectionID: "c1", Conn: d}

	plan, err := d.Explain("SELECT 1 AS n", explain.ModePlain)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}

	rs, err := s.ExecuteExplainPlan(context.Background(), plan, cursor.ScanOptions{})
	if err != nil {
		t.Fatalf("ExecuteExplainPlan: %v", err)
	}
	if len(rs.Rows) == 0 {
		t.Fatal("expected at least one row of plan XML, got none")
	}
	body := rs.Rows[0][0].Text
	if !strings.Contains(body, "<ShowPlanXML") {
		t.Fatalf("expected showplan xml, got: %s", body)
	}
}
