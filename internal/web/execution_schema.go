package web

import (
	"context"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/execution"
)

// runtimeSchemaInspector preserves the schema service's cache boundary while
// moving every live driver operation behind execution.Runtime.
type runtimeSchemaInspector struct {
	runtime execution.SessionRuntime
	handle  execution.SessionHandle
	spec    metadata.SchemaSpec
}

func (i runtimeSchemaInspector) SchemaSpec() metadata.SchemaSpec { return i.spec }

func (i runtimeSchemaInspector) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	return i.runtime.SchemaDirectory(ctx, execution.SchemaRequest{Handle: i.handle, Scope: opts.Root})
}

func (i runtimeSchemaInspector) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	return i.runtime.SchemaObjects(ctx, execution.SchemaRequest{Handle: i.handle, Refs: refs})
}

func (i runtimeSchemaInspector) InspectRelationshipsInScope(ctx context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	return i.runtime.SchemaRelationships(ctx, execution.SchemaRequest{Handle: i.handle, Scope: scope})
}

func (i runtimeSchemaInspector) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	return i.runtime.SchemaDefinition(ctx, execution.SchemaRequest{Handle: i.handle, Ref: &ref})
}

var (
	_ metadata.SchemaInspector       = runtimeSchemaInspector{}
	_ metadata.RelationshipInspector = runtimeSchemaInspector{}
	_ metadata.DefinitionInspector   = runtimeSchemaInspector{}
)
