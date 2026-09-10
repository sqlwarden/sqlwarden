package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

// TestCatalogProceduresAndTriggersExist is a compile-time/signature check;
// behavioral coverage against pg_catalog/information_schema lives in the
// integration test added alongside the live-database wiring task.
func TestCatalogProceduresAndTriggersExist(t *testing.T) {
	_ = CatalogProcedures
	_ = CatalogTriggers
}

// TestCatalogTypesDomainsForeignTablesExist is a compile-time/signature
// check; behavioral coverage against pg_catalog/information_schema lives in
// the integration test added alongside the live-database wiring task.
func TestCatalogTypesDomainsForeignTablesExist(t *testing.T) {
	_ = CatalogTypes
	_ = CatalogDomains
	_ = CatalogForeignTables
}

func TestSchemaSpecIncludesProcedureAndTrigger(t *testing.T) {
	d := &Driver{}
	spec := d.SchemaSpec()

	byKind := map[string]metadata.SchemaObjectKind{}
	for _, k := range spec.Kinds {
		byKind[k.Kind] = k
	}

	proc, ok := byKind["procedure"]
	if !ok {
		t.Fatal("expected SchemaSpec to include a procedure kind")
	}
	if proc.Order != 6 || proc.Relational || proc.SupportsDiagram || proc.Listing != "enumerated" || proc.Label != "Procedure" || proc.PluralLabel != "Procedures" {
		t.Fatalf("unexpected procedure kind shape: %+v", proc)
	}

	trig, ok := byKind["trigger"]
	if !ok {
		t.Fatal("expected SchemaSpec to include a trigger kind")
	}
	if trig.Order != 7 || trig.Relational || trig.SupportsDiagram || trig.Listing != "enumerated" || trig.Label != "Trigger" || trig.PluralLabel != "Triggers" {
		t.Fatalf("unexpected trigger kind shape: %+v", trig)
	}

	if proc.Order >= trig.Order {
		t.Fatalf("expected procedure to sort before trigger, got orders %d and %d", proc.Order, trig.Order)
	}
}

func TestSchemaSpecIncludesTypeDomainAndForeignTable(t *testing.T) {
	d := &Driver{}
	spec := d.SchemaSpec()

	byKind := map[string]metadata.SchemaObjectKind{}
	for _, k := range spec.Kinds {
		byKind[k.Kind] = k
	}

	typ, ok := byKind["type"]
	if !ok {
		t.Fatal("expected SchemaSpec to include a type kind")
	}
	if typ.Order != 8 || typ.Relational || typ.SupportsDiagram || typ.Listing != "enumerated" || typ.Label != "Type" || typ.PluralLabel != "Types" {
		t.Fatalf("unexpected type kind shape: %+v", typ)
	}

	dom, ok := byKind["domain"]
	if !ok {
		t.Fatal("expected SchemaSpec to include a domain kind")
	}
	if dom.Order != 9 || dom.Relational || dom.SupportsDiagram || dom.Listing != "enumerated" || dom.Label != "Domain" || dom.PluralLabel != "Domains" {
		t.Fatalf("unexpected domain kind shape: %+v", dom)
	}

	ft, ok := byKind["foreign_table"]
	if !ok {
		t.Fatal("expected SchemaSpec to include a foreign_table kind")
	}
	if ft.Order != 10 || !ft.Relational || ft.SupportsDiagram || ft.Listing != "enumerated" || ft.Label != "Foreign Table" || ft.PluralLabel != "Foreign Tables" {
		t.Fatalf("unexpected foreign_table kind shape: %+v", ft)
	}

	if typ.Order >= dom.Order || dom.Order >= ft.Order {
		t.Fatalf("expected type < domain < foreign_table order, got %d, %d, %d", typ.Order, dom.Order, ft.Order)
	}
}
