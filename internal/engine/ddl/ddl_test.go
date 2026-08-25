package ddl

import (
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func testSpec() Spec {
	return Spec{
		Operations:               []Operation{OperationCreateTable, OperationDropObject, OperationDropScope, OperationRenameColumn, OperationDropColumn, OperationDropIndex},
		ColumnTypes:              []string{"integer", "text"},
		CreatableTableScopeKinds: []string{"schema"},
		DroppableObjectKinds:     []string{"table", "view"},
		DroppableScopeKinds:      []string{"schema"},
		SupportsCascade:          true,
	}
}

func testScope() metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"})
}

func TestValidateCreateTable(t *testing.T) {
	request := Request{Operation: OperationCreateTable, Scope: testScope(), Name: "events", Columns: []ColumnDefinition{{Name: "id", DataType: "INTEGER", PrimaryKey: true}, {Name: "note", DataType: "text", Nullable: true}}}
	if err := Validate(request, testSpec()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsMalformedRequests(t *testing.T) {
	table := metadata.ObjectRef{Scope: testScope(), Kind: "table", Name: "events"}
	tests := []struct {
		name    string
		request Request
	}{
		{name: "unsupported operation", request: Request{Operation: "alter_database"}},
		{name: "missing columns", request: Request{Operation: OperationCreateTable, Scope: testScope(), Name: "events"}},
		{name: "duplicate columns", request: Request{Operation: OperationCreateTable, Scope: testScope(), Name: "events", Columns: []ColumnDefinition{{Name: "ID", DataType: "integer"}, {Name: "id", DataType: "integer"}}}},
		{name: "raw data type", request: Request{Operation: OperationCreateTable, Scope: testScope(), Name: "events", Columns: []ColumnDefinition{{Name: "id", DataType: "text); drop table users; --"}}}},
		{name: "wrong ref kind", request: Request{Operation: OperationDropColumn, Ref: &metadata.ObjectRef{Scope: testScope(), Kind: "view", Name: "events"}, Name: "id"}},
		{name: "same rename", request: Request{Operation: OperationRenameColumn, Ref: &table, Name: "id", NewName: "id"}},
		{name: "unsupported object", request: Request{Operation: OperationDropObject, Ref: &metadata.ObjectRef{Scope: testScope(), Kind: "function", Name: "f"}}},
		{name: "wrong engine scope", request: Request{Operation: OperationDropColumn, Ref: &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}), Kind: "table", Name: "events"}, Name: "id"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Validate(tt.request, testSpec()); err == nil {
				t.Fatal("Validate() expected error")
			}
		})
	}
}

func TestValidateUnsupportedCascade(t *testing.T) {
	spec := testSpec()
	spec.SupportsCascade = false
	ref := metadata.ObjectRef{Scope: testScope(), Kind: "table", Name: "events"}
	err := Validate(Request{Operation: OperationDropObject, Ref: &ref, Cascade: true}, spec)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Validate() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateIdentifierRejectsWhitespaceAndNUL(t *testing.T) {
	for _, value := range []string{"", " users", "users ", "user\x00name"} {
		if err := ValidateIdentifier(value, "name"); err == nil {
			t.Fatalf("ValidateIdentifier(%q) expected error", value)
		}
	}
}

func TestSummary(t *testing.T) {
	table := metadata.ObjectRef{Scope: testScope(), Kind: "table", Name: "events"}
	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{"create table", Request{Operation: OperationCreateTable, Name: "events"}, "CREATE TABLE events"},
		{"drop object", Request{Operation: OperationDropObject, Ref: &table}, "DROP TABLE events"},
		{"drop scope", Request{Operation: OperationDropScope, Scope: testScope()}, "DROP public"},
		{"rename column", Request{Operation: OperationRenameColumn, Ref: &table, Name: "id", NewName: "customer_id"}, "RENAME COLUMN events.id TO customer_id"},
		{"drop column", Request{Operation: OperationDropColumn, Ref: &table, Name: "id"}, "DROP COLUMN events.id"},
		{"drop index", Request{Operation: OperationDropIndex, Name: "events_id_idx"}, "DROP INDEX events_id_idx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.request.Summary(); got != tt.want {
				t.Fatalf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}
