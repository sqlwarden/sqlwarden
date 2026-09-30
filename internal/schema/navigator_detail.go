package schema

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func (n *Navigator) Objects(ctx context.Context, conn Connection, live metadata.ObjectInspector, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	found := make(map[metadata.ObjectRef]metadata.Object, len(refs))
	var misses []metadata.ObjectRef
	n.mu.Lock()
	mem := n.memoryFor(conn.ID)
	for _, ref := range refs {
		if obj, ok := mem.objects[ref]; ok {
			found[ref] = obj
		} else {
			misses = append(misses, ref)
		}
	}
	n.mu.Unlock()

	if len(misses) > 0 && conn.Persistent && n.store != nil {
		keys := make([]database.SchemaObjectKey, 0, len(misses))
		for _, ref := range misses {
			keys = append(keys, schemaObjectKey(ref))
		}
		rows, err := n.store.SchemaObjects(ctx, conn.ID, keys)
		if err != nil {
			return nil, fmt.Errorf("read cached objects: %w", err)
		}
		var warmed []metadata.Object
		for _, row := range rows {
			var obj metadata.Object
			if err := gunzipJSON(row.ObjectData, &obj); err != nil {
				return nil, fmt.Errorf("decode cached object: %w", err)
			}
			found[obj.Ref] = obj
			warmed = append(warmed, obj)
		}
		n.rememberObjects(conn.ID, warmed)
		misses = remainingRefs(misses, found)
	}

	if len(misses) > 0 {
		if live == nil {
			return nil, ErrSessionRequired
		}
		inspected, err := live.InspectObjects(ctx, misses)
		if err != nil {
			return nil, err
		}
		if err := n.saveObjects(ctx, conn, inspected); err != nil {
			return nil, err
		}
		for _, obj := range inspected {
			found[obj.Ref] = obj
		}
	}

	out := make([]metadata.Object, 0, len(found))
	for _, ref := range refs {
		if obj, ok := found[ref]; ok {
			out = append(out, obj)
		}
	}
	return out, nil
}

func (n *Navigator) Relationships(ctx context.Context, conn Connection, live metadata.RelationshipInspector, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	n.mu.Lock()
	graph, ok := n.memoryFor(conn.ID).relationships[scope]
	n.mu.Unlock()
	if ok {
		return graph, nil
	}
	if conn.Persistent && n.store != nil {
		row, found, err := n.store.SchemaRelationship(ctx, conn.ID, string(scope))
		if err != nil {
			return nil, fmt.Errorf("read cached relationships: %w", err)
		}
		if found {
			var cached metadata.RelationshipGraph
			if err := gunzipJSON(row.Data, &cached); err != nil {
				return nil, fmt.Errorf("decode cached relationships: %w", err)
			}
			n.rememberRelationships(conn.ID, scope, &cached)
			return &cached, nil
		}
	}
	if live == nil {
		return nil, ErrSessionRequired
	}
	graph, err := live.InspectRelationshipsInScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	n.rememberRelationships(conn.ID, scope, graph)
	if conn.Persistent && n.store != nil {
		data, err := gzipJSON(graph)
		if err != nil {
			return nil, fmt.Errorf("encode relationships: %w", err)
		}
		row := database.SchemaRelationship{ConnectionID: conn.ID, Scope: string(scope), Data: data, FetchedAt: n.now().UTC()}
		if err := n.store.UpsertSchemaRelationship(ctx, row); err != nil {
			return nil, fmt.Errorf("store relationships: %w", err)
		}
	}
	return graph, nil
}

func (n *Navigator) Invalidate(ctx context.Context, conn Connection, root metadata.ScopePath) error {
	return n.invalidateDetail(ctx, conn, root)
}

func (n *Navigator) rememberObjects(connID int64, objects []metadata.Object) {
	n.mu.Lock()
	defer n.mu.Unlock()
	mem := n.memoryFor(connID)
	for _, obj := range objects {
		mem.objects[obj.Ref] = obj
	}
}

func (n *Navigator) rememberRelationships(connID int64, scope metadata.ScopePath, graph *metadata.RelationshipGraph) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.memoryFor(connID).relationships[scope] = graph
}

func (n *Navigator) saveObjects(ctx context.Context, conn Connection, objects []metadata.Object) error {
	n.rememberObjects(conn.ID, objects)
	if !conn.Persistent || n.store == nil || len(objects) == 0 {
		return nil
	}
	fetchedAt := n.now().UTC()
	rows := make([]database.SchemaObject, 0, len(objects))
	for _, obj := range objects {
		data, err := gzipJSON(obj)
		if err != nil {
			return fmt.Errorf("encode object: %w", err)
		}
		key := schemaObjectKey(obj.Ref)
		rows = append(rows, database.SchemaObject{ConnectionID: conn.ID, Scope: key.Scope, Kind: key.Kind, Name: key.Name, ObjectData: data, FetchedAt: fetchedAt})
	}
	if err := n.store.UpsertSchemaObjects(ctx, rows); err != nil {
		return fmt.Errorf("store objects: %w", err)
	}
	return nil
}

func schemaObjectKey(ref metadata.ObjectRef) database.SchemaObjectKey {
	return database.SchemaObjectKey{Scope: string(ref.Scope), Kind: ref.Kind, Name: ref.Name}
}

func remainingRefs(refs []metadata.ObjectRef, found map[metadata.ObjectRef]metadata.Object) []metadata.ObjectRef {
	out := refs[:0:0]
	for _, ref := range refs {
		if _, ok := found[ref]; !ok {
			out = append(out, ref)
		}
	}
	return out
}
