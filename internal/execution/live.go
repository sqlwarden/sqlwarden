package execution

import (
	"context"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/schema"
)

type live struct {
	md    Metadata
	scope Scope
	id    SessionID
	tree  metadata.Tree
}

// NewLive binds one session to the schema navigator's Live view. Folder
// loaders never cross the runtime boundary: only the node and folder kind
// strings are sent, resolved from tree against the first parent.
func NewLive(md Metadata, scope Scope, id SessionID, tree metadata.Tree) schema.Live {
	return &live{md: md, scope: scope, id: id, tree: tree}
}

func (l *live) LoadChildren(ctx context.Context, database string, folder metadata.Folder, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	// All parents in one batch share a node kind, so the first names it.
	return l.md.LoadChildren(ctx, l.scope, l.id, ChildrenRequest{
		NodeKind:   l.tree.NodeKindOf(parents[0]),
		FolderKind: folder.Kind,
		Database:   database,
		Parents:    parents,
	})
}

func (l *live) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	return l.md.InspectObjects(ctx, l.scope, l.id, refs)
}

func (l *live) InspectRelationships(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	return l.md.InspectRelationships(ctx, l.scope, l.id, scope)
}

func (l *live) CurrentScope(ctx context.Context) (metadata.ScopePath, error) {
	return l.md.CurrentScope(ctx, l.scope, l.id)
}
