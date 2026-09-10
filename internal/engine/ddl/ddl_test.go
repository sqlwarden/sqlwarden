package ddl

import (
	"errors"
	"strings"
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

func TestSpecCanonicalColumnTypeCustomTypes(t *testing.T) {
	spec := Spec{
		ColumnTypes:              []string{"text", "integer"},
		ParameterizedColumnTypes: []ParameterizedColumnType{{Name: "numeric", Parameters: []ColumnTypeParameter{{Name: "precision", Min: 1, Max: 1000}}}},
		AllowCustomColumnTypes:   true,
	}
	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{"fixed type", "text", "text", true},
		{"valid parameterized", "numeric(10)", "numeric(10)", true},
		{"out-of-range parameterized rejected, not treated as custom", "numeric(1001)", "", false},
		{"unknown extension type accepted", "vector(1536)", "vector(1536)", true},
		{"schema-qualified extension type accepted", "public.hstore", "public.hstore", true},
		{"multi-word extension type accepted", "geometry(Point, 4326)", "geometry(Point, 4326)", true},
		{"injection attempt rejected", "text); drop table users; --", "", false},
		{"quote injection rejected", "text' OR '1'='1", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := spec.CanonicalColumnType(tt.value)
			if ok != tt.ok || got != tt.want {
				t.Errorf("CanonicalColumnType(%q) = (%q, %v), want (%q, %v)", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSpecCanonicalColumnTypeCustomTypesDisabledByDefault(t *testing.T) {
	spec := Spec{ColumnTypes: []string{"text"}}
	if _, ok := spec.CanonicalColumnType("vector(1536)"); ok {
		t.Error("expected unknown type to be rejected when AllowCustomColumnTypes is false")
	}
}

func TestValidCustomColumnTypeSyntax(t *testing.T) {
	valid := []string{"vector", "vector(1536)", "public.hstore", "geometry(Point, 4326)", "character varying", "int4[]"}
	for _, value := range valid {
		if !ValidCustomColumnTypeSyntax(value) {
			t.Errorf("ValidCustomColumnTypeSyntax(%q) = false, want true", value)
		}
	}
	invalid := []string{"", "text); drop table users; --", "text' OR '1'='1", "a;b", strings.Repeat("a", 129)}
	for _, value := range invalid {
		if ValidCustomColumnTypeSyntax(value) {
			t.Errorf("ValidCustomColumnTypeSyntax(%q) = true, want false", value)
		}
	}
}

func TestTableEditsRespectCapabilities(t *testing.T) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "APP"})
	ref := &metadata.ObjectRef{Scope: scope, Kind: "table", Name: "T"}
	spec := Spec{Operations: []Operation{OperationCreateTable, OperationAddColumn, OperationAlterColumn, OperationCreateIndex}, ColumnTypes: []string{"INTEGER"}, CreatableTableScopeKinds: []string{"schema"}}
	value := "0"
	requests := []Request{
		{Operation: OperationCreateTable, Scope: scope, Name: "T", Columns: []ColumnDefinition{{Name: "A", DataType: "INTEGER", Default: &value}}},
		{Operation: OperationAddColumn, Ref: ref, Column: &ColumnDefinition{Name: "A", DataType: "INTEGER", Default: &value}},
		{Operation: OperationAlterColumn, Ref: ref, Name: "A", Changes: &ColumnChanges{Default: &value}},
	}
	for _, request := range requests {
		if err := Validate(request, spec); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s must reject defaults without capability: %v", request.Operation, err)
		}
		enabled := spec
		enabled.SupportsColumnDefaults = true
		if err := Validate(request, enabled); err != nil {
			t.Errorf("%s with defaults enabled: %v", request.Operation, err)
		}
		disabled := enabled
		disabled.Operations = nil
		if err := Validate(request, disabled); !errors.Is(err, ErrUnsupported) {
			t.Errorf("%s must reject unadvertised operations: %v", request.Operation, err)
		}
	}
}
