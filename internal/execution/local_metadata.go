package execution

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
)

// LoadChildren lists one folder for a batch of parents. The database-scoped
// Querier it opens is used only inside this call and never leaves the runtime.
func (r *LocalRuntime) LoadChildren(ctx context.Context, scope Scope, id SessionID, req ChildrenRequest) (map[metadata.ScopePath][]metadata.Child, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return nil, err
	}
	inspector, ok := sess.Conn.(metadata.SchemaInspector)
	if !ok {
		return nil, ErrSchemaUnsupported
	}
	folder, ok := inspector.Tree().Folder(req.NodeKind, req.FolderKind)
	if !ok || folder.List == nil {
		return nil, ErrUnknownFolder
	}
	q, err := inspector.Querier(ctx, req.Database)
	if err != nil {
		return nil, mapError(err, opMetadata)
	}
	children, err := folder.List(ctx, q, req.Parents)
	if err != nil {
		return nil, mapError(err, opMetadata)
	}
	return children, nil
}

func (r *LocalRuntime) InspectObjects(ctx context.Context, scope Scope, id SessionID, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return nil, err
	}
	inspector, ok := sess.Conn.(metadata.ObjectInspector)
	if !ok {
		return nil, ErrSchemaUnsupported
	}
	objects, err := inspector.InspectObjects(ctx, refs)
	if err != nil {
		return nil, mapError(err, opMetadata)
	}
	return objects, nil
}

func (r *LocalRuntime) InspectRelationships(ctx context.Context, scope Scope, id SessionID, path metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return nil, err
	}
	inspector, ok := sess.Conn.(metadata.RelationshipInspector)
	if !ok {
		return nil, ErrRelationshipsUnsupported
	}
	graph, err := inspector.InspectRelationshipsInScope(ctx, path)
	if err != nil {
		return nil, mapError(err, opMetadata)
	}
	return graph, nil
}

func (r *LocalRuntime) InspectDefinition(ctx context.Context, scope Scope, id SessionID, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return nil, err
	}
	inspector, ok := sess.Conn.(metadata.DefinitionInspector)
	if !ok {
		return nil, ErrDefinitionUnsupported
	}
	desc, err := inspector.InspectDefinition(ctx, ref)
	if err != nil {
		return nil, mapError(err, opMetadata)
	}
	return desc, nil
}

// CurrentScope reports the session's current scope. Drivers that cannot
// report one yield an empty path and no error, as do lookups that fail while
// the caller's context is still live; only cancellation is surfaced.
func (r *LocalRuntime) CurrentScope(ctx context.Context, scope Scope, id SessionID) (metadata.ScopePath, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return "", err
	}
	scoper, ok := sess.Conn.(metadata.SessionScoper)
	if !ok {
		return "", nil
	}
	path, err := scoper.CurrentScope(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", mapError(ctxErr, opMetadata)
		}
		return "", nil
	}
	return path, nil
}

// Capabilities returns the driver's specs by value so callers never need the
// driver itself.
func (r *LocalRuntime) Capabilities(ctx context.Context, scope Scope, id SessionID) (SessionCapabilities, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return SessionCapabilities{}, err
	}
	var caps SessionCapabilities
	if inspector, ok := sess.Conn.(metadata.SchemaInspector); ok {
		tree := inspector.Tree()
		caps.Schema = &tree
	}
	if executor, ok := sess.Conn.(ddl.Executor); ok {
		spec := executor.DDLSpec()
		caps.DDL = &spec
	}
	if generator, ok := sess.Conn.(statement.Generator); ok {
		spec := generator.StatementSpec()
		caps.Statements = &spec
	}
	_, caps.Objects = sess.Conn.(metadata.ObjectInspector)
	_, caps.Relationships = sess.Conn.(metadata.RelationshipInspector)
	_, caps.Definitions = sess.Conn.(metadata.DefinitionInspector)
	return caps, nil
}

// ApplyDDL validates req against the driver's spec and applies it under the
// session lock. Validation failures match ErrInvalidDDL.
func (r *LocalRuntime) ApplyDDL(ctx context.Context, scope Scope, id SessionID, req ddl.Request) (TxStatus, error) {
	sess, err := r.session(scope, id)
	if err != nil {
		return TxStatus{}, err
	}
	executor, ok := sess.Conn.(ddl.Executor)
	if !ok {
		return TxStatus{}, mapError(fmt.Errorf("apply ddl: %w", ddl.ErrUnsupported), opDDL)
	}
	if err := ddl.Validate(req, executor.DDLSpec()); err != nil {
		return TxStatus{}, fmt.Errorf("%w: %w", ErrInvalidDDL, err)
	}
	if err := sess.ApplyDDL(ctx, req); err != nil {
		return TxStatus{}, mapError(err, opDDL)
	}
	return txSnapshot(sess), nil
}
