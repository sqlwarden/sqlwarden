package schema

import (
	"context"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

// Live is the caller's live session as the navigator sees it. The navigator
// never opens connections or holds driver handles; every read of the target
// goes through this interface. A nil Live means there is no live session.
type Live interface {
	LoadChildren(ctx context.Context, database string, folder metadata.Folder, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error)
	InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error)
	InspectRelationships(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error)
	CurrentScope(ctx context.Context) (metadata.ScopePath, error)
}
