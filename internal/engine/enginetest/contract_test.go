package enginetest_test

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"
)

type selectOneDriver struct{}

func (selectOneDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (selectOneDriver) Ping(context.Context) error                             { return nil }
func (selectOneDriver) Close() error                                           { return nil }
func (selectOneDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{Rows: []result.Row{{}}}, nil
}
func (selectOneDriver) Execute(context.Context, string, ...any) (*result.ResultSet, error) {
	return &result.ResultSet{}, nil
}
func (selectOneDriver) Dialect() engine.Dialect { return engine.DialectSQLite }

type navigatorDriver struct{ selectOneDriver }

func (navigatorDriver) Tree() metadata.Tree {
	list := func(_ context.Context, _ metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
		out := map[metadata.ScopePath][]metadata.Child{}
		for _, p := range parents {
			out[p] = []metadata.Child{{Kind: "table", Name: "t_" + p.Name("schema")}}
		}
		return out, nil
	}
	return metadata.Tree{
		Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
			{Kind: "schemas", Label: "Schemas", Child: "schema", List: list},
		}},
		Nodes: map[string]metadata.Node{
			"schema": {Label: "Schema", Icon: "schema", Folders: []metadata.Folder{{Kind: "tables", Label: "Tables", Child: "table", List: list}}},
			"table":  {Label: "Table", Icon: "table", Leaf: true},
		},
	}
}

func (navigatorDriver) Querier(context.Context, string) (metadata.Querier, error) {
	return nil, nil
}

func (navigatorDriver) InspectObjects(context.Context, []metadata.ObjectRef) ([]metadata.Object, error) {
	return nil, nil
}

func TestHarnessAcceptsValidEngine(t *testing.T) {
	engine.Register(engine.Registration{
		ID: "harness-fake", DisplayName: "Harness Fake", Dialect: engine.DialectSQLite,
		New: func() engine.Driver { return selectOneDriver{} },
	})
	enginetest.RunCapabilityContract(t, "harness-fake")
	enginetest.RunConnectionContract(t, "harness-fake", engine.ConnectionConfig{DSN: "ignored", Driver: "harness-fake"})
}

func TestNavigatorContractGroupsByParent(t *testing.T) {
	enginetest.RunNavigatorContract(t, navigatorDriver{}, "", []enginetest.NavigatorCase{{
		NodeKind: "schema",
		Folder:   "tables",
		Parents: []metadata.ScopePath{
			metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "a"}),
			metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "b"}),
		},
	}})
}

func TestCapabilityContractValidatesTree(t *testing.T) {
	engine.Register(engine.Registration{ID: "navfake", DisplayName: "Nav", Dialect: engine.DialectSQLite, New: func() engine.Driver { return navigatorDriver{} }})
	enginetest.RunCapabilityContract(t, "navfake")
	set, _ := engine.Describe("navfake")
	if !set.Capabilities[engine.CapabilitySchemaNavigator] || set.Tree == nil {
		t.Fatal("navigator capability not derived")
	}
}
