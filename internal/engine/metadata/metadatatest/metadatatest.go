// Package metadatatest builds fully listed completion views for tests.
package metadatatest

import (
	"fmt"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

// Tree returns a minimal navigator grammar: databases, optionally schemas
// (nested), then relations with columns, functions, and sequences.
func Tree(nested bool) metadata.Tree {
	objectFolders := []metadata.Folder{
		{Kind: "tables", Label: "Tables", Child: "table"},
		{Kind: "views", Label: "Views", Child: "view"},
		{Kind: "materialized_views", Label: "Materialized Views", Child: "materialized_view"},
		{Kind: "foreign_tables", Label: "Foreign Tables", Child: "foreign_table"},
		{Kind: "functions", Label: "Functions", Child: "function"},
		{Kind: "sequences", Label: "Sequences", Child: "sequence"},
	}
	columns := []metadata.Folder{{Kind: "columns", Label: "Columns", Child: "column"}}
	database := metadata.Node{Label: "Database", Scope: true, ShowAllDatabases: true, Folders: objectFolders}
	nodes := map[string]metadata.Node{
		"table":             {Label: "Table", Relational: true, Folders: columns},
		"view":              {Label: "View", Relational: true, Folders: columns},
		"materialized_view": {Label: "Materialized View", Relational: true, Folders: columns},
		"foreign_table":     {Label: "Foreign Table", Relational: true, Folders: columns},
		"column":            {Label: "Column", Leaf: true, Column: true},
		"function":          {Label: "Function", Leaf: true},
		"sequence":          {Label: "Sequence", Leaf: true},
	}
	if nested {
		database.Folders = []metadata.Folder{{Kind: "schemas", Label: "Schemas", Child: "schema"}}
		nodes["schema"] = metadata.Node{Label: "Schema", Scope: true, Folders: objectFolders}
	}
	nodes["database"] = database
	return metadata.Tree{
		Root:  metadata.Node{Label: "Connection", Folders: []metadata.Folder{{Kind: "databases", Label: "Databases", Child: "database"}}},
		Nodes: nodes,
	}
}

// Fixture describes a world that is completely listed: every folder of every
// fixture scope is listed, holding exactly the given objects and refs.
type Fixture struct {
	DefaultScope metadata.ScopePath
	Scopes       []metadata.ScopePath
	Objects      []metadata.Object
	Refs         []metadata.ObjectRef
}

func Build(tree metadata.Tree, f Fixture) *metadata.CompletionView {
	listings := map[metadata.ListingKey][]metadata.Child{}
	objects := map[metadata.ObjectRef]metadata.Object{}
	add := func(parent metadata.ScopePath, kind, name string) {
		node, ok := tree.Node(tree.NodeKindOf(parent))
		if !ok {
			panic(fmt.Sprintf("metadatatest: unknown node for %q", parent))
		}
		for _, folder := range node.Folders {
			if !folder.Contains(kind) {
				continue
			}
			key := metadata.ListingKey{Parent: parent, Folder: folder.Kind}
			for _, existing := range listings[key] {
				if existing.Kind == kind && existing.Name == name {
					return
				}
			}
			listings[key] = append(listings[key], metadata.Child{Kind: kind, Name: name})
			return
		}
		panic(fmt.Sprintf("metadatatest: no folder under %q holds %q", parent, kind))
	}
	listScope := func(path metadata.ScopePath) {
		chain := append(path.Prefixes(), path)
		for i := 1; i < len(chain); i++ {
			last, _ := chain[i].Last()
			add(chain[i-1], last.Kind, last.Name)
		}
		node, _ := tree.Node(tree.NodeKindOf(path))
		for _, folder := range node.Folders {
			key := metadata.ListingKey{Parent: path, Folder: folder.Kind}
			if _, ok := listings[key]; !ok {
				listings[key] = []metadata.Child{}
			}
		}
	}
	for _, scope := range f.Scopes {
		listScope(scope)
	}
	for _, object := range f.Objects {
		listScope(object.Ref.Scope)
		add(object.Ref.Scope, object.Ref.Kind, object.Ref.Name)
		objects[object.Ref] = object
	}
	for _, ref := range f.Refs {
		listScope(ref.Scope)
		add(ref.Scope, ref.Kind, ref.Name)
	}
	if f.DefaultScope != "" {
		listScope(f.DefaultScope)
	}
	return metadata.NewCompletionView(tree, f.DefaultScope, "", listings, objects)
}
