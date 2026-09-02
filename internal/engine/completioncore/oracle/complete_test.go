package oracle

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/completioncore"
)

// fakeResolver is a minimal MetadataResolver with one schema and one table.
type fakeResolver struct{}

func (fakeResolver) DefaultDatabase() string { return "" }
func (fakeResolver) DefaultSchema() string   { return "HR" }
func (fakeResolver) DatabaseNames() []string { return nil }
func (fakeResolver) SchemaNames(string) []string {
	return []string{"HR"}
}
func (fakeResolver) Relations(_, _ string) []completioncore.Relation {
	return []completioncore.Relation{fakeResolver{}.emp()}
}
func (fakeResolver) emp() completioncore.Relation {
	return completioncore.Relation{
		Schema: "HR", Name: "EMPLOYEES", Kind: completioncore.CandidateTable,
		Columns: []completioncore.Column{{Name: "EMPLOYEE_ID"}, {Name: "FIRST_NAME"}},
	}
}
func (f fakeResolver) FindRelation(_, _, name string) (completioncore.Relation, bool) {
	if strings.EqualFold(name, "EMPLOYEES") || strings.EqualFold(name, "EMP") {
		return f.emp(), true
	}
	return completioncore.Relation{}, false
}

func labels(cands []completioncore.Candidate) map[string]completioncore.CandidateType {
	m := map[string]completioncore.CandidateType{}
	for _, c := range cands {
		m[strings.ToUpper(c.Text)] = c.Type
	}
	return m
}

func TestOracleCompleteKeywordsAfterBareStart(t *testing.T) {
	cands, _, err := Complete(context.Background(), "SEL", 3, fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if _, ok := labels(cands)["SELECT"]; !ok {
		t.Fatalf("expected SELECT keyword candidate, got %v", labels(cands))
	}
}

func TestOracleCompleteColumnsForVisibleTable(t *testing.T) {
	const sql = "SELECT  FROM EMPLOYEES"
	cands, cursorCtx, err := Complete(context.Background(), sql, len("SELECT "), fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := labels(cands)
	if got["EMPLOYEE_ID"] != completioncore.CandidateColumn || got["FIRST_NAME"] != completioncore.CandidateColumn {
		t.Fatalf("expected column candidates, got %v", got)
	}
	if cursorCtx.Position != completioncore.PositionColumn && cursorCtx.Position != completioncore.PositionAny {
		t.Errorf("unexpected position %q", cursorCtx.Position)
	}
}

func TestOracleCompletePrefixFilter(t *testing.T) {
	const sql = "SELECT FIRST FROM EMPLOYEES"
	cands, _, err := Complete(context.Background(), sql, len("SELECT FIRST"), fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	for _, c := range cands {
		if c.Type == completioncore.CandidateColumn && !strings.HasPrefix(strings.ToUpper(c.Text), "FIRST") {
			t.Errorf("prefix filter leaked %q", c.Text)
		}
	}
}

func TestOracleCompleteCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Complete(ctx, "SELECT 1 FROM dual", 3, fakeResolver{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}
