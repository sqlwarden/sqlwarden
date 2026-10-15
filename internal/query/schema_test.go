package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/execution"
)

const (
	navigatorDriver = "navigator"
	bareDriver      = "bare"
)

var (
	mainScope = metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	usersRef  = metadata.ObjectRef{Scope: mainScope, Kind: "table", Name: "users"}
)

// schemaDirectory reports the test connection with a configurable driver.
type schemaDirectory struct {
	fakeDirectory
	driver string
}

func (d schemaDirectory) GetConnection(ctx context.Context, id int64) (access.Connection, bool, error) {
	conn, found, err := d.fakeDirectory.GetConnection(ctx, id)
	conn.Driver = d.driver
	return conn, found, err
}

type fakeSchemaStore struct{}

func (fakeSchemaStore) GetConnection(_ context.Context, id int64) (database.Connection, bool, error) {
	if id != testConn {
		return database.Connection{}, false, nil
	}
	return database.Connection{ID: id, DefaultScope: mainScope}, true, nil
}

func (fakeSchemaStore) SchemaSnapshotsEnabled(context.Context, int64) (bool, error) {
	return false, nil
}

type fakeGenerator struct{}

func (fakeGenerator) StatementSpec() statement.Spec {
	return statement.Spec{Objects: []statement.ObjectSpec{{Kind: "table", Operations: []statement.Operation{statement.OperationSelect}}}}
}

func (fakeGenerator) Generate(req statement.Request) (string, error) {
	if req.Object.Ref.Name == "broken" {
		return "", errors.New("table has no columns")
	}
	return "SELECT * FROM " + req.Object.Ref.Name, nil
}

// fakeEngines describes navigatorDriver with a navigator tree, DDL, and
// statements; every other driver has no schema capabilities.
type fakeEngines struct{}

func (fakeEngines) Describe(driver string) (engine.CapabilitySet, bool) {
	if driver != navigatorDriver {
		return engine.CapabilitySet{}, driver == bareDriver
	}
	tree := metadatatest.Tree(false)
	spec := fakeGenerator{}.StatementSpec()
	return engine.CapabilitySet{Tree: &tree, DDL: &ddl.Spec{Operations: []ddl.Operation{ddl.OperationDropObject}}, Statements: &spec}, true
}

func (fakeEngines) StatementGenerator(driver string) (statement.Generator, bool) {
	return fakeGenerator{}, driver == navigatorDriver
}

// schemaRuntime answers metadata reads for any session the embedded fake
// runtime knows in the caller's scope.
type schemaRuntime struct {
	*fakeRuntime
	caps       execution.SessionCapabilities
	children   []metadata.Child
	objects    []metadata.Object
	loadErr    error
	applyErr   error
	applied    []ddl.Request
	metaCalls  int
	capsCalled int
}

func fullCapabilities() execution.SessionCapabilities {
	tree := metadatatest.Tree(false)
	return execution.SessionCapabilities{
		Schema:        &tree,
		DDL:           &ddl.Spec{Operations: []ddl.Operation{ddl.OperationDropObject}, DroppableObjectKinds: []string{"table"}, CreatableTableScopeKinds: []string{"database"}},
		Objects:       true,
		Relationships: true,
		Definitions:   true,
	}
}

func (f *schemaRuntime) Capabilities(_ context.Context, sc execution.Scope, id execution.SessionID) (execution.SessionCapabilities, error) {
	f.capsCalled++
	if _, err := f.lookup(sc, id); err != nil {
		return execution.SessionCapabilities{}, err
	}
	return f.caps, nil
}

func (f *schemaRuntime) LoadChildren(_ context.Context, sc execution.Scope, id execution.SessionID, req execution.ChildrenRequest) (map[metadata.ScopePath][]metadata.Child, error) {
	f.metaCalls++
	if _, err := f.lookup(sc, id); err != nil {
		return nil, err
	}
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	out := map[metadata.ScopePath][]metadata.Child{}
	for _, parent := range req.Parents {
		out[parent] = f.children
	}
	return out, nil
}

func (f *schemaRuntime) InspectObjects(_ context.Context, sc execution.Scope, id execution.SessionID, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	f.metaCalls++
	if _, err := f.lookup(sc, id); err != nil {
		return nil, err
	}
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	var out []metadata.Object
	for _, object := range f.objects {
		for _, ref := range refs {
			if object.Ref == ref {
				out = append(out, object)
			}
		}
	}
	return out, nil
}

func (f *schemaRuntime) InspectRelationships(_ context.Context, sc execution.Scope, id execution.SessionID, path metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	f.metaCalls++
	if _, err := f.lookup(sc, id); err != nil {
		return nil, err
	}
	return &metadata.RelationshipGraph{Scope: path}, nil
}

func (f *schemaRuntime) InspectDefinition(_ context.Context, sc execution.Scope, id execution.SessionID, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	f.metaCalls++
	if _, err := f.lookup(sc, id); err != nil {
		return nil, err
	}
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return &metadata.Descriptor{Kind: "source", Title: ref.Name}, nil
}

func (f *schemaRuntime) CurrentScope(context.Context, execution.Scope, execution.SessionID) (metadata.ScopePath, error) {
	return mainScope, nil
}

func (f *schemaRuntime) ApplyDDL(_ context.Context, sc execution.Scope, id execution.SessionID, req ddl.Request) (execution.TxStatus, error) {
	f.metaCalls++
	if _, err := f.lookup(sc, id); err != nil {
		return execution.TxStatus{}, err
	}
	if f.applyErr != nil {
		return execution.TxStatus{}, f.applyErr
	}
	f.applied = append(f.applied, req)
	return execution.TxStatus{Mode: execution.TxModeAuto}, nil
}

type schemaHarness struct {
	runtime *schemaRuntime
	svc     *Service
}

func newSchemaHarness(driver string) schemaHarness {
	rt := &schemaRuntime{
		fakeRuntime: newFakeRuntime(),
		caps:        fullCapabilities(),
		children:    []metadata.Child{{Kind: "database", Name: "main"}},
		objects:     []metadata.Object{{Ref: usersRef}},
	}
	return schemaHarness{runtime: rt, svc: New(Deps{
		Requirer:    access.NewRequirer(schemaDirectory{driver: driver}, fakePolicy{}),
		Runtime:     rt,
		SchemaStore: fakeSchemaStore{},
		Engines:     fakeEngines{},
	})}
}

func dropUsers() ddl.Request {
	ref := usersRef
	return ddl.Request{Operation: ddl.OperationDropObject, Ref: &ref}
}

// schemaCalls runs every schema operation and returns each error by name.
func schemaCalls(svc *Service, p access.Principal, ref access.ConnRef, id execution.SessionID) map[string]error {
	ctx := context.Background()
	out := map[string]error{}
	_, out["tree"] = svc.SchemaTree(ctx, p, ref)
	_, out["node"] = svc.SchemaNode(ctx, p, ref, id, "", "databases")
	_, out["refresh"] = svc.RefreshSchema(ctx, p, ref, id, "")
	_, out["objects"] = svc.SchemaObjects(ctx, p, ref, id, []metadata.ObjectRef{usersRef})
	_, out["definition"] = svc.ObjectDefinition(ctx, p, ref, id, usersRef)
	_, out["relationships"] = svc.SchemaRelationships(ctx, p, ref, id, mainScope)
	_, out["statement"] = svc.GenerateStatement(ctx, p, ref, id, statement.OperationSelect, usersRef)
	_, out["ddl"] = svc.ObjectDDL(ctx, p, ref, id, dropUsers())
	return out
}

func TestSchemaOperationsDenyBeforeRuntime(t *testing.T) {
	tests := []struct {
		name string
		p    access.Principal
		ref  access.ConnRef
		want error
	}{
		{name: "non-member", p: principal(outsider), ref: connRef(), want: ErrForbidden},
		{name: "org member without connection permission", p: principal(orgOnlyUser), ref: connRef(), want: ErrForbidden},
		{name: "unknown connection", p: principal(userA), ref: access.ConnRef{OrgID: testOrg, WorkspaceID: testWorkspace, ConnectionID: 404}, want: ErrNotFound},
		{name: "connection in another workspace", p: principal(userA), ref: access.ConnRef{OrgID: testOrg, WorkspaceID: otherWorkspace, ConnectionID: testConn}, want: ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newSchemaHarness(navigatorDriver)
			for op, err := range schemaCalls(h.svc, tt.p, tt.ref, ownerSID) {
				accesstest.AssertDenies(t, err, tt.want, op)
			}
			if h.runtime.capsCalled != 0 || h.runtime.metaCalls != 0 {
				t.Fatalf("capabilities calls = %d, metadata calls = %d, want none on denial", h.runtime.capsCalled, h.runtime.metaCalls)
			}
		})
	}
}

func TestSchemaOperationsPerPermission(t *testing.T) {
	ddlAllowed := map[int64]bool{userA: true, dqlUser: false, dmlUser: false, ddlUser: true}
	for account, canDDL := range ddlAllowed {
		t.Run(fmt.Sprintf("account-%d", account), func(t *testing.T) {
			h := newSchemaHarness(navigatorDriver)
			id := h.addSession(account)
			for op, err := range schemaCalls(h.svc, principal(account), connRef(), id) {
				if op == "ddl" && !canDDL {
					accesstest.AssertForbidden(t, err, op)
					continue
				}
				if err != nil {
					t.Fatalf("account %d %s: %v", account, op, err)
				}
			}
		})
	}
}

func (h schemaHarness) addSession(account int64) execution.SessionID {
	return runtimeHarness{runtime: h.runtime.fakeRuntime}.addSession(account)
}

func TestSchemaReadsWithForeignOrMissingSession(t *testing.T) {
	ctx := context.Background()
	h := newSchemaHarness(navigatorDriver)
	foreign := execution.SessionID("b-session")

	if _, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), foreign, "", "databases"); !errors.Is(err, ErrSchemaSessionExpired) {
		t.Fatalf("node with foreign session err = %v, want ErrSchemaSessionExpired", err)
	}
	if _, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), "", "", "databases"); !errors.Is(err, ErrSchemaSessionRequired) {
		t.Fatalf("node without session err = %v, want ErrSchemaSessionRequired", err)
	}
	if _, err := h.svc.RefreshSchema(ctx, principal(userA), connRef(), foreign, ""); !errors.Is(err, ErrSchemaSessionExpired) {
		t.Fatalf("refresh with foreign session err = %v, want ErrSchemaSessionExpired", err)
	}
	if _, err := h.svc.ObjectDefinition(ctx, principal(userA), connRef(), foreign, usersRef); !errors.Is(err, ErrSchemaSessionExpired) {
		t.Fatalf("definition with foreign session err = %v, want ErrSchemaSessionExpired", err)
	}
	if _, err := h.svc.SchemaObjects(ctx, principal(userA), connRef(), "", []metadata.ObjectRef{usersRef}); !errors.Is(err, ErrSchemaSessionRequired) {
		t.Fatalf("objects without session err = %v, want ErrSchemaSessionRequired", err)
	}
	var failure *execution.Failure
	if _, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), foreign, dropUsers()); !errors.As(err, &failure) || failure.Code != execution.FailureSessionNotFound {
		t.Fatalf("ddl with foreign session err = %v, want session not found failure", err)
	}
	if _, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), "", dropUsers()); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("ddl without session err = %v, want ErrSessionRequired", err)
	}
	if h.runtime.metaCalls != 0 {
		t.Fatalf("metadata calls = %d, want none for a foreign session", h.runtime.metaCalls)
	}
}

func TestSchemaNodeServesFromSessionThenCache(t *testing.T) {
	ctx := context.Background()
	h := newSchemaHarness(navigatorDriver)
	listing, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), ownerSID, "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Items) != 1 || listing.Items[0].Name != "main" {
		t.Fatalf("items = %+v, want main", listing.Items)
	}
	cached, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), "", "", "databases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cached.Items) != 1 || h.runtime.metaCalls != 1 {
		t.Fatalf("cached items = %+v, metadata calls = %d, want cached listing without a reload", cached.Items, h.runtime.metaCalls)
	}
}

func TestSchemaNodeRejections(t *testing.T) {
	ctx := context.Background()
	h := newSchemaHarness(navigatorDriver)
	if _, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), ownerSID, "", ""); !errors.Is(err, ErrSchemaFolderRequired) {
		t.Fatalf("missing folder err = %v", err)
	}
	if _, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), ownerSID, "", "nope"); !errors.Is(err, ErrSchemaUnknownFolder) {
		t.Fatalf("unknown folder err = %v", err)
	}
	h.runtime.loadErr = errors.New("catalog query failed")
	var read *SchemaReadError
	if _, err := h.svc.SchemaNode(ctx, principal(userA), connRef(), ownerSID, "", "databases"); !errors.As(err, &read) {
		t.Fatalf("load failure err = %v, want SchemaReadError", err)
	}

	bare := newSchemaHarness(bareDriver)
	if _, err := bare.svc.SchemaTree(ctx, principal(userA), connRef()); !errors.Is(err, ErrSchemaNavigatorUnsupported) {
		t.Fatalf("tree err = %v, want ErrSchemaNavigatorUnsupported", err)
	}
	if _, err := bare.svc.SchemaNode(ctx, principal(userA), connRef(), ownerSID, "", ""); !errors.Is(err, ErrSchemaNavigatorUnsupported) {
		t.Fatalf("node err = %v, want ErrSchemaNavigatorUnsupported before folder validation", err)
	}
	if _, err := bare.svc.RefreshSchema(ctx, principal(userA), connRef(), ownerSID, ""); !errors.Is(err, ErrSchemaNavigatorUnsupported) {
		t.Fatalf("refresh err = %v, want ErrSchemaNavigatorUnsupported", err)
	}
}

func TestSchemaTreeIncludesSpecs(t *testing.T) {
	h := newSchemaHarness(navigatorDriver)
	spec, err := h.svc.SchemaTree(context.Background(), principal(dqlUser), connRef())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Editor == nil || spec.Statements == nil || len(spec.Tree.Root.Folders) == 0 {
		t.Fatalf("spec = %+v, want tree with editor and statements", spec)
	}
}

func TestSchemaCapabilityGates(t *testing.T) {
	ctx := context.Background()
	h := newSchemaHarness(navigatorDriver)
	h.runtime.caps = execution.SessionCapabilities{}
	if _, err := h.svc.SchemaObjects(ctx, principal(userA), connRef(), ownerSID, []metadata.ObjectRef{usersRef}); !errors.Is(err, execution.ErrSchemaUnsupported) {
		t.Fatalf("objects err = %v, want ErrSchemaUnsupported", err)
	}
	if _, err := h.svc.ObjectDefinition(ctx, principal(userA), connRef(), ownerSID, usersRef); !errors.Is(err, execution.ErrDefinitionUnsupported) {
		t.Fatalf("definition err = %v, want ErrDefinitionUnsupported", err)
	}
	if _, err := h.svc.SchemaRelationships(ctx, principal(userA), connRef(), ownerSID, mainScope); !errors.Is(err, execution.ErrRelationshipsUnsupported) {
		t.Fatalf("relationships err = %v, want ErrRelationshipsUnsupported", err)
	}
	if _, err := h.svc.GenerateStatement(ctx, principal(userA), connRef(), ownerSID, statement.OperationSelect, usersRef); !errors.Is(err, execution.ErrSchemaUnsupported) {
		t.Fatalf("statement err = %v, want ErrSchemaUnsupported", err)
	}
	if _, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), ownerSID, dropUsers()); !errors.Is(err, execution.ErrDDLUnsupported) {
		t.Fatalf("ddl err = %v, want ErrDDLUnsupported", err)
	}
	if h.runtime.metaCalls != 0 {
		t.Fatalf("metadata calls = %d, want none past a capability gate", h.runtime.metaCalls)
	}
}

func TestSchemaReadsReturnSessionMetadata(t *testing.T) {
	ctx := context.Background()
	h := newSchemaHarness(navigatorDriver)
	objects, err := h.svc.SchemaObjects(ctx, principal(userA), connRef(), ownerSID, []metadata.ObjectRef{usersRef})
	if err != nil || len(objects) != 1 || objects[0].Ref != usersRef {
		t.Fatalf("objects = %+v, err = %v", objects, err)
	}
	descriptor, err := h.svc.ObjectDefinition(ctx, principal(userA), connRef(), ownerSID, usersRef)
	if err != nil || descriptor == nil || descriptor.Title != "users" {
		t.Fatalf("descriptor = %+v, err = %v", descriptor, err)
	}
	graph, err := h.svc.SchemaRelationships(ctx, principal(userA), connRef(), ownerSID, mainScope)
	if err != nil || graph == nil || graph.Scope != mainScope {
		t.Fatalf("graph = %+v, err = %v", graph, err)
	}
	h.runtime.loadErr = errors.New("definition query failed")
	var read *SchemaReadError
	if _, err := h.svc.ObjectDefinition(ctx, principal(userA), connRef(), ownerSID, usersRef); !errors.As(err, &read) {
		t.Fatalf("definition failure err = %v, want SchemaReadError", err)
	}
}

func TestGenerateStatement(t *testing.T) {
	ctx := context.Background()
	h := newSchemaHarness(navigatorDriver)
	sql, err := h.svc.GenerateStatement(ctx, principal(dqlUser), connRef(), h.addSession(dqlUser), statement.OperationSelect, usersRef)
	if err != nil || sql != "SELECT * FROM users" {
		t.Fatalf("sql = %q, err = %v", sql, err)
	}
	if _, err := h.svc.GenerateStatement(ctx, principal(userA), connRef(), ownerSID, statement.OperationDelete, usersRef); !errors.Is(err, ErrStatementUnsupported) {
		t.Fatalf("unsupported operation err = %v", err)
	}
	missing := metadata.ObjectRef{Scope: mainScope, Kind: "table", Name: "missing"}
	if _, err := h.svc.GenerateStatement(ctx, principal(userA), connRef(), ownerSID, statement.OperationSelect, missing); !errors.Is(err, ErrSchemaObjectNotFound) {
		t.Fatalf("missing object err = %v", err)
	}
	broken := metadata.ObjectRef{Scope: mainScope, Kind: "table", Name: "broken"}
	h.runtime.objects = append(h.runtime.objects, metadata.Object{Ref: broken})
	var generate *StatementError
	if _, err := h.svc.GenerateStatement(ctx, principal(userA), connRef(), ownerSID, statement.OperationSelect, broken); !errors.As(err, &generate) {
		t.Fatalf("generator failure err = %v, want StatementError", err)
	}

	bare := newSchemaHarness(bareDriver)
	if _, err := bare.svc.GenerateStatement(ctx, principal(userA), connRef(), ownerSID, statement.OperationSelect, usersRef); !errors.Is(err, ErrStatementsUnsupported) {
		t.Fatalf("bare driver err = %v, want ErrStatementsUnsupported", err)
	}
}

func TestObjectDDL(t *testing.T) {
	ctx := context.Background()

	t.Run("applies and refreshes", func(t *testing.T) {
		h := newSchemaHarness(navigatorDriver)
		result, err := h.svc.ObjectDDL(ctx, principal(ddlUser), connRef(), h.addSession(ddlUser), dropUsers())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.runtime.applied) != 1 || result.RefreshFailed || result.Persistent || result.Transaction.Mode != execution.TxModeAuto {
			t.Fatalf("applied = %d, result = %+v", len(h.runtime.applied), result)
		}
	})

	t.Run("refresh without navigator session marks stale", func(t *testing.T) {
		h := newSchemaHarness(navigatorDriver)
		refs := []metadata.ObjectRef{usersRef}
		if _, err := h.svc.SchemaObjects(ctx, principal(userA), connRef(), ownerSID, refs); err != nil {
			t.Fatal(err)
		}
		if _, err := h.svc.SchemaObjects(ctx, principal(userA), connRef(), "", refs); err != nil {
			t.Fatalf("cached object read err = %v, want the object served from cache", err)
		}
		h.runtime.caps.Schema = nil
		result, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), ownerSID, dropUsers())
		if err != nil {
			t.Fatal(err)
		}
		if !result.RefreshFailed || len(h.runtime.applied) != 1 {
			t.Fatalf("result = %+v, want applied with a failed refresh", result)
		}
		if _, err := h.svc.SchemaObjects(ctx, principal(userA), connRef(), "", refs); !errors.Is(err, ErrSchemaSessionRequired) {
			t.Fatalf("object read after failed refresh err = %v, want the cached object invalidated", err)
		}
	})

	t.Run("rejected by the spec", func(t *testing.T) {
		h := newSchemaHarness(navigatorDriver)
		var edit *SchemaEditError
		_, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), ownerSID, ddl.Request{Operation: ddl.OperationCreateTable})
		if !errors.As(err, &edit) || edit.Code != SchemaEditInvalid || len(h.runtime.applied) != 0 {
			t.Fatalf("err = %v, want invalid schema edit before applying", err)
		}
	})

	t.Run("runtime failures", func(t *testing.T) {
		cases := []struct {
			err  error
			code string
		}{
			{err: execution.ErrInvalidDDL, code: SchemaEditInvalid},
			{err: &execution.TargetError{Err: errors.New("table is locked")}, code: SchemaEditFailed},
		}
		for _, c := range cases {
			h := newSchemaHarness(navigatorDriver)
			h.runtime.applyErr = c.err
			var edit *SchemaEditError
			if _, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), ownerSID, dropUsers()); !errors.As(err, &edit) || edit.Code != c.code {
				t.Fatalf("err = %v, want schema edit error %s", err, c.code)
			}
		}
		h := newSchemaHarness(navigatorDriver)
		h.runtime.applyErr = execution.ErrDDLUnsupported
		if _, err := h.svc.ObjectDDL(ctx, principal(userA), connRef(), ownerSID, dropUsers()); !errors.Is(err, execution.ErrDDLUnsupported) {
			t.Fatalf("err = %v, want ErrDDLUnsupported", err)
		}
	})
}
