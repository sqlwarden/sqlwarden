package schema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

// CompletionMetadata projects whatever the navigator has cached for conn into
// the MetadataSet the completion index consumes. Relations whose detail is not
// cached get columns synthesized from their column listings.
func (n *Navigator) CompletionMetadata(ctx context.Context, conn Connection, tree metadata.Tree) (*metadata.MetadataSet, error) {
	listings, err := n.cachedWithin(ctx, conn, "")
	if err != nil {
		return nil, err
	}
	objects, err := n.cachedObjects(ctx, conn)
	if err != nil {
		return nil, err
	}

	keys := make([]listingKey, 0, len(listings))
	for key := range listings {
		keys = append(keys, key)
	}
	// Parent paths sort before their descendants, so hidden system subtrees
	// are recorded before their own listings are visited.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].parent != keys[j].parent {
			return keys[i].parent < keys[j].parent
		}
		return keys[i].folder < keys[j].folder
	})

	scopes := map[metadata.ScopePath]*metadata.ScopeNode{}
	ensureScope := func(path metadata.ScopePath, system bool) *metadata.ScopeNode {
		node, ok := scopes[path]
		if !ok {
			node = &metadata.ScopeNode{Path: path, Groups: []metadata.ObjectGroup{}, System: system}
			scopes[path] = node
		}
		return node
	}
	columns := map[metadata.ObjectRef][]metadata.Column{}
	var hidden []metadata.ScopePath
	for _, key := range keys {
		if withinAny(key.parent, hidden) {
			continue
		}
		parentNode, _ := tree.Node(tree.NodeKindOf(key.parent))
		if parentNode.Scope && key.parent != "" {
			ensureScope(key.parent, false)
		}
		for _, item := range listings[key] {
			if item.System && !conn.ShowSystem {
				hidden = append(hidden, key.parent.Child(metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}))
				continue
			}
			node, ok := tree.Node(item.Kind)
			if !ok {
				continue
			}
			switch {
			case node.Scope:
				ensureScope(key.parent.Child(metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}), item.System)
			case node.Column:
				if ref, ok := metadata.ObjectRefOf(key.parent); ok {
					columns[ref] = append(columns[ref], columnFromChild(item))
				}
			case parentNode.Scope:
				addGroupRef(ensureScope(key.parent, false), metadata.ObjectRef{Scope: key.parent, Kind: item.Kind, Name: item.Name})
			}
		}
	}

	for ref := range objects {
		if withinAny(ref.Path(), hidden) {
			delete(objects, ref)
		}
	}
	for ref, cols := range columns {
		if _, detailed := objects[ref]; detailed {
			continue
		}
		sort.SliceStable(cols, func(i, j int) bool { return cols[i].Ordinal < cols[j].Ordinal })
		objects[ref] = metadata.Object{Ref: ref, Relational: &metadata.RelationalDetail{Columns: cols}}
	}

	directory := &metadata.Directory{DefaultScope: conn.DefaultScope, Roots: make([]metadata.ScopeNode, 0, len(scopes))}
	for _, node := range scopes {
		directory.Roots = append(directory.Roots, *node)
	}
	sort.Slice(directory.Roots, func(i, j int) bool { return directory.Roots[i].Path < directory.Roots[j].Path })

	set := &metadata.MetadataSet{Directory: directory, Objects: make([]metadata.Object, 0, len(objects))}
	for _, obj := range objects {
		set.Objects = append(set.Objects, obj)
	}
	sort.Slice(set.Objects, func(i, j int) bool { return set.Objects[i].Ref.Path() < set.Objects[j].Ref.Path() })

	raw, err := json.Marshal(struct {
		Roots   []metadata.ScopeNode
		Objects []metadata.Object
		Default metadata.ScopePath
	}{directory.Roots, set.Objects, conn.DefaultScope})
	if err != nil {
		return nil, fmt.Errorf("hash completion metadata: %w", err)
	}
	sum := sha256.Sum256(raw)
	set.Version = hex.EncodeToString(sum[:])
	return set, nil
}

func (n *Navigator) cachedObjects(ctx context.Context, conn Connection) (map[metadata.ObjectRef]metadata.Object, error) {
	out := map[metadata.ObjectRef]metadata.Object{}
	if conn.Persistent && n.store != nil {
		rows, err := n.store.AllSchemaObjects(ctx, conn.ID)
		if err != nil {
			return nil, fmt.Errorf("read cached objects: %w", err)
		}
		for _, row := range rows {
			var obj metadata.Object
			if err := gunzipJSON(row.ObjectData, &obj); err != nil {
				return nil, fmt.Errorf("decode cached object: %w", err)
			}
			out[obj.Ref] = obj
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for ref, obj := range n.memoryFor(conn.ID).objects {
		out[ref] = obj
	}
	return out, nil
}

func addGroupRef(node *metadata.ScopeNode, ref metadata.ObjectRef) {
	for i := range node.Groups {
		if node.Groups[i].Kind == ref.Kind {
			node.Groups[i].Objects = append(node.Groups[i].Objects, ref)
			return
		}
	}
	node.Groups = append(node.Groups, metadata.ObjectGroup{Kind: ref.Kind, Objects: []metadata.ObjectRef{ref}})
}

func columnFromChild(item metadata.Child) metadata.Column {
	col := metadata.Column{Name: item.Name}
	if v, ok := item.Attributes["data_type"].(string); ok {
		col.DataType = v
	}
	if v, ok := item.Attributes["nullable"].(bool); ok {
		col.Nullable = v
	}
	col.Ordinal = intAttribute(item.Attributes["ordinal"])
	return col
}

// intAttribute accepts the integer types loaders produce and the float64 a
// JSON round trip through the store yields.
func intAttribute(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}
