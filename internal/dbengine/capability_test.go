package dbengine

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine/dbsql"
	"github.com/sqlwarden/internal/dbengine/schema"
)

// schemaCursorDriver implements the optional schema + cursor interfaces so we
// can assert derivation turns them into capabilities.
type schemaCursorDriver struct{ fakeDriver }

func (schemaCursorDriver) SchemaSpec() schema.SchemaSpec {
	return schema.SchemaSpec{Dialect: "postgres", Kinds: []schema.SchemaObjectKind{{Kind: "table"}}}
}
func (schemaCursorDriver) InspectCatalog(context.Context, schema.CatalogOptions) (*schema.Catalog, error) {
	return &schema.Catalog{}, nil
}
func (schemaCursorDriver) InspectObjects(context.Context, []schema.ObjectRef) ([]schema.Object, error) {
	return nil, nil
}
func (schemaCursorDriver) StartQuery(context.Context, dbsql.QueryRequest) (dbsql.QueryCursor, error) {
	return nil, nil
}

func TestCapabilitiesDerivedFromInterfaces(t *testing.T) {
	resetRegistry(t)
	Register(Registration{
		ID: "postgres", DisplayName: "PostgreSQL", Dialect: DialectPostgres,
		New: func() Driver { return schemaCursorDriver{} },
	})
	set, ok := Describe("postgres")
	if !ok {
		t.Fatal("Describe should find the registered engine")
	}

	if set.Engine.ID != "postgres" {
		t.Fatalf("descriptor ID = %q", set.Engine.ID)
	}
	if !set.Capabilities[CapabilitySchemaCatalog] || !set.Capabilities[CapabilitySchemaObjects] {
		t.Errorf("schema caps should be true: %+v", set.Capabilities)
	}
	if !set.Capabilities[CapabilityQueryCursor] {
		t.Errorf("query.cursor should be true (driver implements StartQuery)")
	}
	if set.Schema == nil || len(set.Schema.Kinds) != 1 {
		t.Errorf("schema spec should be populated from SchemaSpec(): %+v", set.Schema)
	}
	// SQL features are derived from interfaces too: this fake implements none.
	if set.Capabilities[CapabilitySQLClassify] || set.Capabilities[CapabilitySQLParse] || set.Capabilities[CapabilitySQLRewrite] {
		t.Errorf("sql caps should be false for a driver implementing no SQL feature: %+v", set.Capabilities)
	}
}

func TestCapabilitiesAbsentWhenInterfacesNotImplemented(t *testing.T) {
	resetRegistry(t)
	registerFake("plain", DialectPostgres) // fakeDriver implements neither schema nor cursor
	set, _ := Describe("plain")

	if set.Capabilities[CapabilitySchemaCatalog] || set.Capabilities[CapabilityQueryCursor] {
		t.Errorf("plain driver must not report schema/cursor caps: %+v", set.Capabilities)
	}
	if set.Schema != nil {
		t.Errorf("plain driver must not carry a schema spec: %+v", set.Schema)
	}
}
