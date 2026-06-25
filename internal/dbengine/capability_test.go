package dbengine

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine/dbsql"
	"github.com/sqlwarden/internal/dbengine/schema"
	"github.com/sqlwarden/internal/driver"
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
		NewDriver: func() driver.Driver { return schemaCursorDriver{} },
	})
	eng, _ := New("postgres")
	set := eng.Capabilities()

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
	if !set.Capabilities[CapabilitySQLClassify] {
		t.Errorf("sql.classify should be true (heuristic fallback always classifies)")
	}
}

func TestCapabilitiesAbsentWhenInterfacesNotImplemented(t *testing.T) {
	resetRegistry(t)
	registerFake("plain", DialectPostgres) // fakeDriver implements neither schema nor cursor
	eng, _ := New("plain")
	set := eng.Capabilities()

	if set.Capabilities[CapabilitySchemaCatalog] || set.Capabilities[CapabilityQueryCursor] {
		t.Errorf("plain driver must not report schema/cursor caps: %+v", set.Capabilities)
	}
	if set.Schema != nil {
		t.Errorf("plain driver must not carry a schema spec: %+v", set.Schema)
	}
}
