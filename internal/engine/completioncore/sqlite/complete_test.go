package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/completioncore"
)

// fakeResolver is a minimal MetadataResolver: one database "main" with tables
// users(id,name) and orders(id,user_id).
type fakeResolver struct{}

func (fakeResolver) DefaultDatabase() string     { return "main" }
func (fakeResolver) DefaultSchema() string       { return "" }
func (fakeResolver) DatabaseNames() []string     { return []string{"main"} }
func (fakeResolver) SchemaNames(string) []string { return nil }
func (fakeResolver) Relations(db, schema string) []completioncore.Relation {
	return []completioncore.Relation{
		{Database: "main", Name: "users", Kind: completioncore.CandidateTable, Columns: []completioncore.Column{{Name: "id"}, {Name: "name"}}},
		{Database: "main", Name: "orders", Kind: completioncore.CandidateTable, Columns: []completioncore.Column{{Name: "id"}, {Name: "user_id"}}},
	}
}
func (fakeResolver) FindRelation(db, schema, name string) (completioncore.Relation, bool) {
	for _, r := range (fakeResolver{}).Relations(db, schema) {
		if strings.EqualFold(r.Name, name) {
			return r, true
		}
	}
	return completioncore.Relation{}, false
}

func candidateTexts(cs []completioncore.Candidate) map[string]completioncore.CandidateType {
	m := map[string]completioncore.CandidateType{}
	for _, c := range cs {
		m[strings.ToLower(c.Text)] = c.Type
	}
	return m
}

func TestCompleteRelationsAfterFrom(t *testing.T) {
	sql := "SELECT * FROM "
	cs, cc, err := Complete(context.Background(), sql, len(sql), fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := candidateTexts(cs)
	if got["users"] != completioncore.CandidateTable || got["orders"] != completioncore.CandidateTable {
		t.Fatalf("expected users+orders tables, got %v", got)
	}
	if got["select"] != completioncore.CandidateKeyword {
		t.Fatalf("expected keywords alongside relations, got %v", got)
	}
	if cc.Position != completioncore.PositionRelation {
		t.Fatalf("Position = %q, want relation", cc.Position)
	}
}

func TestCompleteColumnsAfterSelect(t *testing.T) {
	sql := "SELECT  FROM users"
	cs, _, err := Complete(context.Background(), sql, len("SELECT "), fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := candidateTexts(cs)
	if got["id"] != completioncore.CandidateColumn || got["name"] != completioncore.CandidateColumn {
		t.Fatalf("expected id+name columns, got %v", got)
	}
	if got["distinct"] != completioncore.CandidateKeyword {
		t.Fatalf("expected keywords in column position, got %v", got)
	}
	if got["count"] != completioncore.CandidateFunction {
		t.Fatalf("expected functions in column position, got %v", got)
	}
}

func TestCompleteFunctionsAndKeywordsFilterByPrefix(t *testing.T) {
	sql := "SELECT cou FROM users"
	cs, _, err := Complete(context.Background(), sql, len("SELECT cou"), fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := candidateTexts(cs)
	if got["count"] != completioncore.CandidateFunction {
		t.Fatalf("expected count function for prefix 'cou', got %v", got)
	}
	for text := range got {
		if !strings.HasPrefix(text, "cou") {
			t.Fatalf("candidate %q does not match prefix 'cou'", text)
		}
	}
}

func TestCompleteQualifiedColumns(t *testing.T) {
	sql := "SELECT u. FROM users u"
	cs, _, err := Complete(context.Background(), sql, len("SELECT u."), fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := candidateTexts(cs)
	if got["id"] != completioncore.CandidateColumn || got["name"] != completioncore.CandidateColumn {
		t.Fatalf("expected users columns via alias, got %v", got)
	}
	if _, ok := got["user_id"]; ok {
		t.Fatalf("orders columns leaked into u. qualifier")
	}
}

func TestCompleteKeywordsAtStatementStart(t *testing.T) {
	cs, _, err := Complete(context.Background(), "", 0, fakeResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := candidateTexts(cs)
	if got["select"] != completioncore.CandidateKeyword {
		t.Fatalf("expected SELECT keyword at start, got %v", got)
	}
}

func TestCompleteContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Complete(ctx, "SELECT ", 7, fakeResolver{}); err == nil {
		t.Fatalf("expected context error")
	}
}
