package engine

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/safety"
	"github.com/sqlwarden/internal/engine/statement"
)

// capabilityDriver implements the optional metadata, DDL, and cursor interfaces so we
// can assert derivation turns them into capabilities.
type capabilityDriver struct{ fakeDriver }

func (capabilityDriver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{Dialect: "postgres", Kinds: []metadata.SchemaObjectKind{{Kind: "table"}}}
}
func (capabilityDriver) InspectDirectory(context.Context, metadata.DirectoryOptions) (*metadata.Directory, error) {
	return &metadata.Directory{}, nil
}
func (capabilityDriver) InspectObjects(context.Context, []metadata.ObjectRef) ([]metadata.Object, error) {
	return nil, nil
}
func (capabilityDriver) StartQuery(context.Context, cursor.QueryRequest) (cursor.QueryCursor, error) {
	return nil, nil
}
func (capabilityDriver) DDLSpec() ddl.Spec {
	return ddl.Spec{Operations: []ddl.Operation{ddl.OperationCreateTable}}
}
func (capabilityDriver) ApplyDDL(context.Context, ddl.Request) error { return nil }
func (capabilityDriver) StatementSpec() statement.Spec {
	return statement.Spec{Objects: []statement.ObjectSpec{{Kind: "table", Operations: []statement.Operation{statement.OperationSelect}}}}
}
func (capabilityDriver) Generate(statement.Request) (string, error) { return "SELECT 1;", nil }
func (capabilityDriver) Check(context.Context, safety.Request) (safety.Result, error) {
	return safety.Result{Source: "omni"}, nil
}
func (capabilityDriver) ExplainSpec() explain.Spec { return explain.Spec{SupportsAnalyze: true} }
func (capabilityDriver) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
	return explain.Plan{Statement: "EXPLAIN " + sql}, nil
}
func (capabilityDriver) TLSSpec() TLSSpec {
	return TLSSpec{
		Modes:            []TLSMode{TLSModeRequire, TLSModeVerifyFull},
		SupportsCABundle: true,
	}
}
func (capabilityDriver) SupportsSSHTunnel() bool { return true }

func TestCapabilitiesDerivedFromInterfaces(t *testing.T) {
	resetRegistry(t)
	Register(Registration{
		ID: "postgres", DisplayName: "PostgreSQL", Dialect: DialectPostgres,
		New: func() Driver { return capabilityDriver{} },
	})
	set, ok := Describe("postgres")
	if !ok {
		t.Fatal("Describe should find the registered engine")
	}

	if set.Engine.ID != "postgres" {
		t.Fatalf("descriptor ID = %q", set.Engine.ID)
	}
	if !set.Capabilities[CapabilitySchemaDirectory] || !set.Capabilities[CapabilitySchemaObjects] {
		t.Errorf("schema caps should be true: %+v", set.Capabilities)
	}
	if !set.Capabilities[CapabilityQueryCursor] {
		t.Errorf("query.cursor should be true (driver implements StartQuery)")
	}
	if !set.Capabilities[CapabilityDDL] || set.DDL == nil {
		t.Errorf("schema.edit and its spec should be derived from Executor: %+v", set)
	}
	if !set.Capabilities[CapabilitySQLGenerate] || set.Statements == nil {
		t.Errorf("sql.generate and its spec should be derived from Generator: %+v", set)
	}
	if !set.Capabilities[CapabilitySQLSafetyCheck] {
		t.Errorf("sql.safety_check should be true (driver implements Check): %+v", set.Capabilities)
	}
	if !set.Capabilities[CapabilitySQLExplain] || set.Explain == nil || !set.Explain.SupportsAnalyze {
		t.Errorf("sql.explain and its spec should be derived from Explainer: %+v", set)
	}
	if set.Schema == nil || len(set.Schema.Kinds) != 1 {
		t.Errorf("schema spec should be populated from SchemaSpec(): %+v", set.Schema)
	}
	// SQL features are derived from interfaces too: this fake implements none.
	if set.Capabilities[CapabilitySQLClassify] || set.Capabilities[CapabilitySQLParse] || set.Capabilities[CapabilitySQLRewrite] {
		t.Errorf("sql caps should be false for a driver implementing no SQL feature: %+v", set.Capabilities)
	}
	if !set.Capabilities[CapabilityTLS] || set.TLS == nil || len(set.TLS.Modes) != 2 {
		t.Errorf("connection.tls and its spec should be derived from TLSCapable: %+v", set)
	}
	if !set.Capabilities[CapabilitySSHTunnel] {
		t.Errorf("connection.ssh_tunnel should be derived from SSHTunnelCapable: %+v", set.Capabilities)
	}
}

func TestCapabilitiesAbsentWhenInterfacesNotImplemented(t *testing.T) {
	resetRegistry(t)
	registerFake("plain", DialectPostgres) // fakeDriver implements neither schema nor cursor
	set, _ := Describe("plain")

	if set.Capabilities[CapabilitySchemaDirectory] || set.Capabilities[CapabilityQueryCursor] {
		t.Errorf("plain driver must not report schema/cursor caps: %+v", set.Capabilities)
	}
	if set.Capabilities[CapabilitySQLSafetyCheck] {
		t.Errorf("plain driver must not report sql.safety_check: %+v", set.Capabilities)
	}
	if set.Capabilities[CapabilitySQLExplain] || set.Explain != nil {
		t.Errorf("plain driver must not report sql.explain: %+v", set.Capabilities)
	}
	if set.Capabilities[CapabilityTLS] || set.TLS != nil {
		t.Errorf("plain driver must not report connection.tls: %+v", set.Capabilities)
	}
	if set.Capabilities[CapabilitySSHTunnel] {
		t.Errorf("plain driver must not report connection.ssh_tunnel: %+v", set.Capabilities)
	}
	if set.Schema != nil {
		t.Errorf("plain driver must not carry a schema spec: %+v", set.Schema)
	}
}
