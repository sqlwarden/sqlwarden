package oracle

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/completioncore"
	"github.com/sqlwarden/internal/engine/metadata"
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

// schemaAwareResolver has a default schema of APP and distinct relations per
// schema so a completion can be checked against the qualifier that was used.
type schemaAwareResolver struct{}

func (schemaAwareResolver) DefaultDatabase() string     { return "" }
func (schemaAwareResolver) DefaultSchema() string       { return "APP" }
func (schemaAwareResolver) DatabaseNames() []string     { return nil }
func (schemaAwareResolver) SchemaNames(string) []string { return []string{"APP", "HR"} }
func (schemaAwareResolver) Relations(_, schema string) []completioncore.Relation {
	switch strings.ToUpper(schema) {
	case "HR":
		return []completioncore.Relation{{Schema: "HR", Name: "EMPLOYEES", Kind: completioncore.CandidateTable}}
	case "APP":
		return []completioncore.Relation{{Schema: "APP", Name: "ACCOUNTS", Kind: completioncore.CandidateTable}}
	default:
		return nil
	}
}
func (schemaAwareResolver) FindRelation(_, _, _ string) (completioncore.Relation, bool) {
	return completioncore.Relation{}, false
}

func TestOracleCompleteSchemaQualifiedRelations(t *testing.T) {
	const sql = "SELECT * FROM HR."
	cands, _, err := Complete(context.Background(), sql, len(sql), schemaAwareResolver{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	got := labels(cands)
	if got["EMPLOYEES"] != completioncore.CandidateTable {
		t.Fatalf("expected HR.EMPLOYEES from the qualifier schema, got %v", got)
	}
	if _, leaked := got["ACCOUNTS"]; leaked {
		t.Fatalf("default-schema relation leaked past the qualifier: %v", got)
	}
}

func TestOracleCompleteCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Complete(ctx, "SELECT 1 FROM dual", 3, fakeResolver{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestBuiltinFunctionContexts(t *testing.T) {
	for _, sql := range []string{"SELECT NV| FROM EMP", "SELECT * FROM EMP WHERE NV|", "SELECT * FROM EMP ORDER BY NV|", "SELECT COALESCE(NV|, 0) FROM EMP"} {
		cursor := strings.IndexByte(sql, '|')
		query := strings.Replace(sql, "|", "", 1)
		candidates, _, err := Complete(context.Background(), query, cursor, fakeResolver{})
		if err != nil || labels(candidates)["NVL"] != completioncore.CandidateFunction {
			t.Errorf("%s: %v, %v", sql, labels(candidates), err)
		}
		for _, candidate := range candidates {
			if candidate.Text == "NVL" && candidate.InsertText != "NVL" {
				t.Errorf("builtin insertion: %+v", candidate)
			}
		}
	}
	for _, sql := range []string{"SELECT * FROM NV|", "SELECT e.NV| FROM EMP e", "DROP FUNCTION NV|", "SELECT 'NV|' FROM EMP", "SELECT q'[NV|]' FROM EMP", "SELECT 1 -- NV|", "SELECT /* NV| */ 1 FROM EMP"} {
		cursor := strings.IndexByte(sql, '|')
		candidates, _, err := Complete(context.Background(), strings.Replace(sql, "|", "", 1), cursor, fakeResolver{})
		if err != nil {
			t.Fatal(err)
		}
		if labels(candidates)["NVL"] == completioncore.CandidateFunction {
			t.Errorf("builtin leaked into %s", sql)
		}
	}
}

func TestDerivedColumnsAndCTENames(t *testing.T) {
	for _, tt := range []struct{ sql, want, absent string }{
		{"WITH emp AS (SELECT 1 AS local_id FROM dual) SELECT e.| FROM emp e", "LOCAL_ID", "EMPLOYEE_ID"},
		{"SELECT s.| FROM (SELECT 1 AS local_id FROM dual) s", "LOCAL_ID", "EMPLOYEE_ID"},
		{"WITH recent AS (SELECT 1 AS local_id FROM dual) SELECT * FROM rec|", "RECENT", "EMPLOYEE_ID"},
		{"WITH \"Recent Jobs\" AS (SELECT 1 AS \"Job No\" FROM dual) SELECT r.| FROM \"Recent Jobs\" r", "JOB NO", "EMPLOYEE_ID"},
	} {
		cursor := strings.IndexByte(tt.sql, '|')
		candidates, _, err := Complete(context.Background(), strings.Replace(tt.sql, "|", "", 1), cursor, fakeResolver{})
		if err != nil {
			t.Fatal(err)
		}
		got := labels(candidates)
		if _, ok := got[tt.want]; !ok {
			t.Errorf("%s: missing %s: %v", tt.sql, tt.want, got)
		}
		if _, ok := got[tt.absent]; ok {
			t.Errorf("%s: leaked %s", tt.sql, tt.absent)
		}
	}
}

type catalogResolver struct{ fakeResolver }

func (catalogResolver) CatalogObjects(_, schema string, kinds ...string) []metadata.ObjectRef {
	if schema != "" && !strings.EqualFold(schema, "HR") {
		return nil
	}
	var result []metadata.ObjectRef
	for kind, name := range map[string]string{"function": "CALCULATE_TAX", "procedure": "REBUILD_TOTALS", "sequence": "ORDER_SEQ"} {
		if slices.Contains(kinds, kind) {
			result = append(result, metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "HR"}), Kind: kind, Name: name})
		}
	}
	return result
}

func TestCatalogRoutineAndSequenceCompletion(t *testing.T) {
	for _, tt := range []struct{ sql, want string }{
		{"SELECT CALC| FROM EMP", "CALCULATE_TAX"},
		{"SELECT HR.CALC| FROM EMP", "CALCULATE_TAX"},
		{"DROP PROCEDURE REB|", "REBUILD_TOTALS"},
		{"SELECT ORDER_SEQ.| FROM EMP", "NEXTVAL"},
	} {
		cursor := strings.IndexByte(tt.sql, '|')
		candidates, _, err := Complete(context.Background(), strings.Replace(tt.sql, "|", "", 1), cursor, catalogResolver{})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := labels(candidates)[tt.want]; !ok {
			t.Errorf("%s: missing %s: %v", tt.sql, tt.want, labels(candidates))
		}
	}
}

func TestCursorWord(t *testing.T) {
	for _, tt := range []struct {
		sql                string
		cursor, start, end int
		prefix             string
		suppressed         bool
	}{
		{"SELECT NAME FROM EMP", 9, 7, 11, "NA", false},
		{"SELECT \"Job No\" FROM EMP", 12, 7, 15, "Job ", false},
		{"SELECT \"Job No", 14, 7, 14, "Job No", false},
		{"SELECT 'NVL' FROM EMP", 10, 10, 10, "", true},
		{"SELECT -- NV", 12, 12, 12, "", true},
		{"SELECT /* done */ NV", 20, 18, 20, "NV", false},
		{"SELECT", 0, 0, 0, "", false},
	} {
		start, end, prefix, suppressed := CursorWord(tt.sql, tt.cursor)
		if start != tt.start || end != tt.end || prefix != tt.prefix || suppressed != tt.suppressed {
			t.Errorf("%q @ %d: %d %d %q %v; want %d %d %q %v", tt.sql, tt.cursor, start, end, prefix, suppressed, tt.start, tt.end, tt.prefix, tt.suppressed)
		}
	}
}
