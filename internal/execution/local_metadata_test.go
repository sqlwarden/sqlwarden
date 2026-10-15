package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

var mainDB = metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})

func fkSession(t *testing.T) (*LocalRuntime, SessionInfo) {
	t.Helper()
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)
	for _, stmt := range []string{
		"create table parent (id integer primary key, name text)",
		"create table child (id integer primary key, parent_id integer references parent(id))",
	} {
		if _, err := r.Execute(context.Background(), baseScope, ExecuteRequest{SessionID: info.ID, SQL: stmt}); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	return r, info
}

func TestMetadataLoadChildren(t *testing.T) {
	r, info := fkSession(t)
	ctx := context.Background()

	roots, err := r.LoadChildren(ctx, baseScope, info.ID, ChildrenRequest{
		FolderKind: "databases",
		Parents:    []metadata.ScopePath{""},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range roots[""] {
		found = found || c.Name == "main"
	}
	if !found {
		t.Fatalf("root databases missing main: %+v", roots)
	}

	tables, err := r.LoadChildren(ctx, baseScope, info.ID, ChildrenRequest{
		NodeKind:   "database",
		FolderKind: "tables",
		Database:   "main",
		Parents:    []metadata.ScopePath{mainDB},
	})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range tables[mainDB] {
		names[c.Name] = true
	}
	if !names["parent"] || !names["child"] {
		t.Fatalf("tables: %+v", tables)
	}
}

func TestMetadataLoadChildrenUnknownFolder(t *testing.T) {
	r, info := fkSession(t)
	_, err := r.LoadChildren(context.Background(), baseScope, info.ID, ChildrenRequest{NodeKind: "database", FolderKind: "nope"})
	if !errors.Is(err, ErrUnknownFolder) {
		t.Fatalf("want ErrUnknownFolder, got %v", err)
	}
	_, err = r.LoadChildren(context.Background(), baseScope, info.ID, ChildrenRequest{NodeKind: "bogus", FolderKind: "tables"})
	if !errors.Is(err, ErrUnknownFolder) {
		t.Fatalf("unknown node: want ErrUnknownFolder, got %v", err)
	}
}

func TestMetadataInspectObjects(t *testing.T) {
	r, info := fkSession(t)
	objs, err := r.InspectObjects(context.Background(), baseScope, info.ID, []metadata.ObjectRef{{Scope: mainDB, Kind: "table", Name: "parent"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Relational == nil || len(objs[0].Relational.Columns) != 2 {
		t.Fatalf("objects: %+v", objs)
	}
}

func TestMetadataInspectRelationships(t *testing.T) {
	r, info := fkSession(t)
	graph, err := r.InspectRelationships(context.Background(), baseScope, info.ID, mainDB)
	if err != nil {
		t.Fatal(err)
	}
	if graph == nil || len(graph.Relationships) == 0 {
		t.Fatalf("expected an FK edge, got %+v", graph)
	}
	edge := graph.Relationships[0]
	if edge.Source.Name != "child" || edge.References.Name != "parent" {
		t.Fatalf("edge: %+v", edge)
	}
}

func TestMetadataInspectDefinition(t *testing.T) {
	r, info := fkSession(t)
	desc, err := r.InspectDefinition(context.Background(), baseScope, info.ID, metadata.ObjectRef{Scope: mainDB, Kind: "table", Name: "parent"})
	if err != nil || desc == nil {
		t.Fatalf("definition: %+v %v", desc, err)
	}
}

func TestMetadataCurrentScope(t *testing.T) {
	r, info := fkSession(t)
	path, err := r.CurrentScope(context.Background(), baseScope, info.ID)
	if err != nil || path != mainDB {
		t.Fatalf("scope: %q %v", path, err)
	}
}

func TestMetadataCapabilities(t *testing.T) {
	r, info := fkSession(t)
	caps, err := r.Capabilities(context.Background(), baseScope, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if caps.Schema == nil || caps.DDL == nil || caps.Statements == nil || !caps.Objects || !caps.Relationships || !caps.Definitions {
		t.Fatalf("caps: %+v", caps)
	}
	if len(caps.DDL.Operations) == 0 {
		t.Fatal("ddl spec empty")
	}
}

func TestMetadataApplyDDL(t *testing.T) {
	r, info := fkSession(t)
	ctx := context.Background()
	req := ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     mainDB,
		Name:      "made",
		Columns:   []ddl.ColumnDefinition{{Name: "id", DataType: "integer", PrimaryKey: true}},
	}
	st, err := r.ApplyDDL(ctx, baseScope, info.ID, req)
	if err != nil || st.Mode != TxModeAuto || st.Open || st.Statements == nil {
		t.Fatalf("apply: %+v %v", st, err)
	}
	res, err := r.Query(ctx, baseScope, QueryRequest{SessionID: info.ID, SQL: "select count(*) from made"})
	if err != nil || len(res.Result.Rows) != 1 {
		t.Fatalf("created table not queryable: %v", err)
	}

	bad := ddl.Request{Operation: ddl.OperationCreateTable, Scope: mainDB, Name: "x", Columns: []ddl.ColumnDefinition{{Name: "id", DataType: "bogus"}}}
	if _, err := r.ApplyDDL(ctx, baseScope, info.ID, bad); !errors.Is(err, ErrInvalidDDL) {
		t.Fatalf("want ErrInvalidDDL, got %v", err)
	}
}

func TestMetadataApplyDDLCancelledIsOutcomeUnknown(t *testing.T) {
	r, info := fkSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     mainDB,
		Name:      "late",
		Columns:   []ddl.ColumnDefinition{{Name: "id", DataType: "integer"}},
	}
	_, err := r.ApplyDDL(ctx, baseScope, info.ID, req)
	requireFailure(t, err, FailureExecutionOutcomeUnknown)
	if !errors.Is(err, ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
		t.Fatalf("sentinels lost: %v", err)
	}
}

func TestMetadataReadCancelledIsNotOutcomeUnknown(t *testing.T) {
	r, info := fkSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.InspectObjects(ctx, baseScope, info.ID, []metadata.ObjectRef{{Scope: mainDB, Kind: "table", Name: "parent"}})
	var f *Failure
	if errors.As(err, &f) {
		t.Fatalf("metadata read must not map to a failure code: %v", err)
	}
}

func TestMetadataForeignScope(t *testing.T) {
	r, info := fkSession(t)
	ctx := context.Background()
	other := scopeOf("o1", "w1", "a2", "c1")

	_, err := r.LoadChildren(ctx, other, info.ID, ChildrenRequest{FolderKind: "databases"})
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.InspectObjects(ctx, other, info.ID, nil)
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.InspectRelationships(ctx, other, info.ID, mainDB)
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.InspectDefinition(ctx, other, info.ID, metadata.ObjectRef{})
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.CurrentScope(ctx, other, info.ID)
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.Capabilities(ctx, other, info.ID)
	requireFailure(t, err, FailureSessionNotFound)
	_, err = r.ApplyDDL(ctx, other, info.ID, ddl.Request{})
	requireFailure(t, err, FailureSessionNotFound)
}

type bareInspectorDriver struct{ engine.Driver }

func (bareInspectorDriver) Connect(context.Context, engine.ConnectionConfig) error { return nil }
func (bareInspectorDriver) Close() error                                           { return nil }
func (bareInspectorDriver) Tree() metadata.Tree {
	return metadata.Tree{Root: metadata.Node{Folders: []metadata.Folder{{Kind: "empty", Child: "thing"}}}}
}
func (bareInspectorDriver) Querier(context.Context, string) (metadata.Querier, error) {
	return nil, nil
}
func (bareInspectorDriver) InspectObjects(context.Context, []metadata.ObjectRef) ([]metadata.Object, error) {
	return nil, nil
}

func init() {
	engine.Register(engine.Registration{
		ID:          "bareinspector",
		DisplayName: "Bare inspector",
		Dialect:     engine.DialectSQLite,
		New:         func() engine.Driver { return bareInspectorDriver{} },
	})
}

func bareSession(t *testing.T) (*LocalRuntime, SessionInfo) {
	t.Helper()
	r, p := newTestRuntime(t, allowAll())
	p.creds = credentials.Credentials{Driver: "bareinspector", DSN: "x"}
	return r, mustOpen(t, r, baseScope)
}

func TestMetadataFolderWithoutLoader(t *testing.T) {
	r, info := bareSession(t)
	_, err := r.LoadChildren(context.Background(), baseScope, info.ID, ChildrenRequest{FolderKind: "empty"})
	if !errors.Is(err, ErrUnknownFolder) {
		t.Fatalf("want ErrUnknownFolder, got %v", err)
	}
}

func TestMetadataUnsupportedCapabilities(t *testing.T) {
	r, info := bareSession(t)
	ctx := context.Background()
	if _, err := r.ApplyDDL(ctx, baseScope, info.ID, ddl.Request{}); !errors.Is(err, ErrDDLUnsupported) {
		t.Fatalf("want ErrDDLUnsupported, got %v", err)
	}
	if _, err := r.InspectRelationships(ctx, baseScope, info.ID, mainDB); !errors.Is(err, ErrRelationshipsUnsupported) {
		t.Fatalf("relationships: %v", err)
	}
	if _, err := r.InspectDefinition(ctx, baseScope, info.ID, metadata.ObjectRef{}); !errors.Is(err, ErrDefinitionUnsupported) {
		t.Fatalf("definition: %v", err)
	}
	if path, err := r.CurrentScope(ctx, baseScope, info.ID); err != nil || path != "" {
		t.Fatalf("scope: %q %v", path, err)
	}
	caps, err := r.Capabilities(ctx, baseScope, info.ID)
	if err != nil || caps.DDL != nil || caps.Statements != nil || caps.Relationships || caps.Definitions || caps.Schema == nil {
		t.Fatalf("caps: %+v %v", caps, err)
	}
}
