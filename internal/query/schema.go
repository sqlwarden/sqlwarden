package query

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/schema"
)

var (
	ErrSchemaNavigatorUnsupported = errors.New("query: schema navigator is not supported by this driver")
	ErrSchemaFolderRequired       = errors.New("query: schema folder is required")
	ErrSchemaUnknownFolder        = errors.New("query: schema folder is not defined for this node")
	// ErrSchemaSessionRequired reports a read the cache cannot answer without
	// a live session.
	ErrSchemaSessionRequired = errors.New("query: schema read requires a live session")
	// ErrSchemaSessionExpired reports a read that named a session which is not
	// live in the caller's scope.
	ErrSchemaSessionExpired = errors.New("query: schema session has expired or does not exist")
	ErrSchemaObjectNotFound = errors.New("query: schema object was not found")
)

// SchemaReadError reports metadata that could not be loaded from the cache or
// the target.
type SchemaReadError struct {
	Err error
}

func (e *SchemaReadError) Error() string { return e.Err.Error() }
func (e *SchemaReadError) Unwrap() error { return e.Err }

// Schema metadata, DDL, and statement types the schema methods accept and
// return. Transports name them through this package so they need not depend on
// the engine packages.
type (
	ScopePath          = metadata.ScopePath
	ScopeSegment       = metadata.ScopeSegment
	ObjectRef          = metadata.ObjectRef
	SchemaObject       = metadata.Object
	ObjectDescriptor   = metadata.Descriptor
	RelationshipGraph  = metadata.RelationshipGraph
	Tree               = metadata.Tree
	DDLRequest         = ddl.Request
	DDLSpec            = ddl.Spec
	StatementOperation = statement.Operation
	StatementSpec      = statement.Spec
)

// SchemaTreeSpec is the navigator tree of a driver together with the editor
// and statement specs that accompany it.
type SchemaTreeSpec struct {
	Tree       metadata.Tree
	Editor     *ddl.Spec
	Statements *statement.Spec
}

// schemaSession is a live session together with what its driver supports.
type schemaSession struct {
	id   execution.SessionID
	caps execution.SessionCapabilities
}

// schemaRequest is an authorized schema read on one connection. session is
// nil when the caller named no session or one that is not live.
type schemaRequest struct {
	conn      access.Connection
	scope     execution.Scope
	requested execution.SessionID
	session   *schemaSession
}

// authorizeSchema checks that the principal may use a live session on the
// connection, which is also what reading its schema requires.
func (s *Service) authorizeSchema(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID) (*schemaRequest, error) {
	conn, _, err := s.runtimeConnection(ctx, p, ref)
	if err != nil {
		return nil, err
	}
	return &schemaRequest{conn: conn, scope: scope(p, ref), requested: id}, nil
}

// resolveSession binds the named session when it is live in the caller's
// scope. A missing, expired, or foreign session is not an error: cached reads
// still succeed and uncached reads report session required or expired.
func (s *Service) resolveSession(ctx context.Context, req *schemaRequest) error {
	if req.requested == "" {
		return nil
	}
	caps, err := s.deps.Runtime.Capabilities(ctx, req.scope, req.requested)
	if err != nil {
		var failure *execution.Failure
		if errors.As(err, &failure) {
			return nil
		}
		return err
	}
	req.session = &schemaSession{id: req.requested, caps: caps}
	return nil
}

// sessionLive binds the session to the navigator. A driver without a
// navigator tree still serves object and relationship reads.
func (s *Service) sessionLive(req *schemaRequest) schema.Live {
	if req.session == nil {
		return nil
	}
	var tree metadata.Tree
	if req.session.caps.Schema != nil {
		tree = *req.session.caps.Schema
	}
	return execution.NewLive(s.deps.Runtime, req.scope, req.session.id, tree)
}

// navigatorLive binds the session only when its driver has a navigator tree.
func (s *Service) navigatorLive(req *schemaRequest) schema.Live {
	if req.session == nil || req.session.caps.Schema == nil {
		return nil
	}
	return s.sessionLive(req)
}

// inspectsSchema rejects a live session whose driver cannot inspect objects.
// Without a session the read falls back to cached metadata.
func (req *schemaRequest) inspectsSchema() error {
	if req.session == nil || req.session.caps.Objects {
		return nil
	}
	return execution.ErrSchemaUnsupported
}

// sessionRequired distinguishes a caller that named a dead session, who must
// drop it, from one that named none.
func (s *Service) sessionRequired(ctx context.Context, req *schemaRequest) error {
	if req.requested != "" {
		if _, err := s.deps.Runtime.Get(ctx, req.scope, req.requested); err != nil {
			return ErrSchemaSessionExpired
		}
	}
	return ErrSchemaSessionRequired
}

func (s *Service) schemaReadError(ctx context.Context, req *schemaRequest, err error) error {
	switch {
	case errors.Is(err, schema.ErrSessionRequired):
		return s.sessionRequired(ctx, req)
	case errors.Is(err, schema.ErrUnknownFolder), errors.Is(err, execution.ErrUnknownFolder):
		return ErrSchemaUnknownFolder
	}
	return &SchemaReadError{Err: err}
}

func (s *Service) navigatorTree(driver string) (metadata.Tree, bool) {
	set, ok := s.deps.Engines.Describe(driver)
	if !ok || set.Tree == nil {
		return metadata.Tree{}, false
	}
	return *set.Tree, true
}

// SchemaTree returns the connection driver's navigator tree. It reads no
// session because the tree is static per driver.
func (s *Service) SchemaTree(ctx context.Context, p access.Principal, ref access.ConnRef) (SchemaTreeSpec, error) {
	conn, _, err := s.runtimeConnection(ctx, p, ref)
	if err != nil {
		return SchemaTreeSpec{}, err
	}
	set, ok := s.deps.Engines.Describe(conn.Driver)
	if !ok || set.Tree == nil {
		return SchemaTreeSpec{}, ErrSchemaNavigatorUnsupported
	}
	return SchemaTreeSpec{Tree: *set.Tree, Editor: set.DDL, Statements: set.Statements}, nil
}

// SchemaNode lists one folder under parent, from the cache or the session.
func (s *Service) SchemaNode(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, parent metadata.ScopePath, folder string) (Listing, error) {
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return Listing{}, err
	}
	tree, ok := s.navigatorTree(req.conn.Driver)
	if !ok {
		return Listing{}, ErrSchemaNavigatorUnsupported
	}
	if folder == "" {
		return Listing{}, ErrSchemaFolderRequired
	}
	if err := s.resolveSession(ctx, req); err != nil {
		return Listing{}, err
	}
	nav, err := s.navigatorConnection(ctx, req.conn)
	if err != nil {
		return Listing{}, err
	}
	listing, err := s.deps.Navigator.Children(ctx, nav, tree, s.navigatorLive(req), parent, folder)
	if err != nil {
		return Listing{}, s.schemaReadError(ctx, req, err)
	}
	return listing, nil
}

// RefreshSchema reloads every cached listing under root from the session.
func (s *Service) RefreshSchema(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, root metadata.ScopePath) ([]Listing, error) {
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return nil, err
	}
	tree, ok := s.navigatorTree(req.conn.Driver)
	if !ok {
		return nil, ErrSchemaNavigatorUnsupported
	}
	if err := s.resolveSession(ctx, req); err != nil {
		return nil, err
	}
	live := s.navigatorLive(req)
	if live == nil {
		return nil, s.sessionRequired(ctx, req)
	}
	nav, err := s.navigatorConnection(ctx, req.conn)
	if err != nil {
		return nil, err
	}
	listings, err := s.deps.Navigator.Refresh(ctx, nav, tree, live, root)
	if err != nil {
		return nil, s.schemaReadError(ctx, req, err)
	}
	return listings, nil
}

// SchemaObjects describes the referenced objects, from the cache or the
// session.
func (s *Service) SchemaObjects(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return nil, err
	}
	return s.schemaObjects(ctx, req, refs)
}

func (s *Service) schemaObjects(ctx context.Context, req *schemaRequest, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	if err := s.resolveSession(ctx, req); err != nil {
		return nil, err
	}
	if err := req.inspectsSchema(); err != nil {
		return nil, err
	}
	nav, err := s.navigatorConnection(ctx, req.conn)
	if err != nil {
		return nil, err
	}
	objects, err := s.deps.Navigator.Objects(ctx, nav, s.sessionLive(req), refs)
	if err != nil {
		return nil, s.schemaReadError(ctx, req, err)
	}
	return objects, nil
}

// ObjectDefinition loads an object's definition from the session. It needs a
// live session because definitions are never cached.
func (s *Service) ObjectDefinition(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, object metadata.ObjectRef) (*metadata.Descriptor, error) {
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return nil, err
	}
	if err := s.resolveSession(ctx, req); err != nil {
		return nil, err
	}
	if req.session == nil {
		return nil, s.sessionRequired(ctx, req)
	}
	if !req.session.caps.Definitions {
		return nil, execution.ErrDefinitionUnsupported
	}
	descriptor, err := s.deps.Runtime.InspectDefinition(ctx, req.scope, req.session.id, object)
	if err != nil {
		return nil, s.schemaReadError(ctx, req, err)
	}
	return descriptor, nil
}

// SchemaRelationships returns the relationship graph of one scope, from the
// cache or the session.
func (s *Service) SchemaRelationships(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, path metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	req, err := s.authorizeSchema(ctx, p, ref, id)
	if err != nil {
		return nil, err
	}
	if err := s.resolveSession(ctx, req); err != nil {
		return nil, err
	}
	if req.session != nil && !req.session.caps.Relationships {
		return nil, execution.ErrRelationshipsUnsupported
	}
	nav, err := s.navigatorConnection(ctx, req.conn)
	if err != nil {
		return nil, err
	}
	graph, err := s.deps.Navigator.Relationships(ctx, nav, s.sessionLive(req), path)
	if err != nil {
		return nil, s.schemaReadError(ctx, req, err)
	}
	return graph, nil
}
