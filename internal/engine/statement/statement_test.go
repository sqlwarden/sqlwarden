package statement

import (
	"errors"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestValidate(t *testing.T) {
	spec := Spec{Objects: []ObjectSpec{
		{Kind: "table", Operations: []Operation{OperationSelect, OperationInsert}},
		{Kind: "view", Operations: []Operation{OperationSelect}},
	}}
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	object := metadata.Object{
		Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "orders"},
		Relational: &metadata.RelationalDetail{Columns: []metadata.Column{
			{Name: "id", Ordinal: 1},
			{Name: "note", Ordinal: 2},
		}},
	}

	if err := Validate(Request{Operation: OperationInsert, Object: object}, spec); err != nil {
		t.Fatalf("valid request: %v", err)
	}

	unsupported := object
	unsupported.Ref.Kind = "view"
	if err := Validate(Request{Operation: OperationDelete, Object: unsupported}, spec); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported operation error = %v", err)
	}

	missingDetail := object
	missingDetail.Relational = nil
	if err := Validate(Request{Operation: OperationSelect, Object: missingDetail}, spec); err == nil {
		t.Fatal("expected missing relational detail to fail")
	}

	emptyColumn := object
	emptyColumn.Relational = &metadata.RelationalDetail{Columns: []metadata.Column{{Name: ""}}}
	if err := Validate(Request{Operation: OperationSelect, Object: emptyColumn}, spec); err == nil {
		t.Fatal("expected empty column name to fail")
	}
}

func TestOperationsForReturnsCopy(t *testing.T) {
	spec := Spec{Objects: []ObjectSpec{{Kind: "table", Operations: []Operation{OperationSelect}}}}
	operations := spec.OperationsFor("table")
	operations[0] = OperationDelete
	if !spec.Supports("table", OperationSelect) {
		t.Fatal("caller mutated statement spec")
	}
}

func TestBuildUsesSafeWritePredicates(t *testing.T) {
	columns := []string{`"id"`, `"note"`}
	values := []string{"$1", "$2"}

	update, err := Build(OperationUpdate, `"public"."orders"`, columns, values)
	if err != nil {
		t.Fatal(err)
	}
	if update != "UPDATE \"public\".\"orders\"\nSET\n  \"id\" = $1,\n  \"note\" = $2\nWHERE 1 = 0;" {
		t.Fatalf("unexpected UPDATE:\n%s", update)
	}

	deleteSQL, err := Build(OperationDelete, `"public"."orders"`, columns, values)
	if err != nil {
		t.Fatal(err)
	}
	if deleteSQL != "DELETE FROM \"public\".\"orders\"\nWHERE 1 = 0;" {
		t.Fatalf("unexpected DELETE:\n%s", deleteSQL)
	}

	if _, err := Build(OperationInsert, `"public"."orders"`, columns, values[:1]); err == nil {
		t.Fatal("expected placeholder mismatch to fail")
	}
}
