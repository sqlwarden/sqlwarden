package schema

import (
	"context"
	"errors"

	"github.com/sqlwarden/internal/engine/metadata"
)

// ErrInspectorCapabilityUnsupported reports an optional metadata capability the
// wrapped inspector does not implement.
var ErrInspectorCapabilityUnsupported = errors.New("schema: inspector does not support this capability")

type inspectorLive struct {
	inspector metadata.SchemaInspector
}

// LiveFromInspector adapts a connected inspector, such as the short-lived one
// handed out by a connection probe, to Live. Optional capabilities the
// inspector lacks report ErrInspectorCapabilityUnsupported, except
// CurrentScope, which yields an empty path. A nil inspector yields a nil Live
// so callers keep the navigator's no-session behavior.
func LiveFromInspector(inspector metadata.SchemaInspector) Live {
	if inspector == nil {
		return nil
	}
	return inspectorLive{inspector: inspector}
}

func (l inspectorLive) LoadChildren(ctx context.Context, database string, folder metadata.Folder, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	q, err := l.inspector.Querier(ctx, database)
	if err != nil {
		return nil, err
	}
	return folder.List(ctx, q, parents)
}

func (l inspectorLive) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	inspector, ok := l.inspector.(metadata.ObjectInspector)
	if !ok {
		return nil, ErrInspectorCapabilityUnsupported
	}
	return inspector.InspectObjects(ctx, refs)
}

func (l inspectorLive) InspectRelationships(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	inspector, ok := l.inspector.(metadata.RelationshipInspector)
	if !ok {
		return nil, ErrInspectorCapabilityUnsupported
	}
	return inspector.InspectRelationshipsInScope(ctx, scope)
}

func (l inspectorLive) CurrentScope(ctx context.Context) (metadata.ScopePath, error) {
	scoper, ok := l.inspector.(metadata.SessionScoper)
	if !ok {
		return "", nil
	}
	return scoper.CurrentScope(ctx)
}
