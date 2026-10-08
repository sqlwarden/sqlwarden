package schema

import (
	"context"
	"fmt"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

// DiscoverScopes lists the non-system scope nodes (databases, schemas, ...)
// directly under parent for the connection form's default-scope pickers. When
// the driver flags a current scope, discovery descends one level into it so a
// single test connection yields both database and schema choices.
func DiscoverScopes(ctx context.Context, tree metadata.Tree, live Live, parent metadata.ScopePath) (metadata.ScopeDiscovery, error) {
	discovery := metadata.ScopeDiscovery{Current: parent, Scopes: []metadata.ScopePath{}}
	scopes, current, err := scopeChildren(ctx, tree, live, parent)
	if err != nil {
		return metadata.ScopeDiscovery{}, err
	}
	discovery.Scopes = append(discovery.Scopes, scopes...)
	if current == "" {
		return discovery, nil
	}
	discovery.Current = current
	nested, nestedCurrent, err := scopeChildren(ctx, tree, live, current)
	if err != nil {
		return metadata.ScopeDiscovery{}, err
	}
	discovery.Scopes = append(discovery.Scopes, nested...)
	if nestedCurrent != "" {
		discovery.Current = nestedCurrent
	}
	return discovery, nil
}

func scopeChildren(ctx context.Context, tree metadata.Tree, live Live, parent metadata.ScopePath) ([]metadata.ScopePath, metadata.ScopePath, error) {
	node, ok := tree.Node(tree.NodeKindOf(parent))
	if !ok {
		return nil, "", fmt.Errorf("discover scopes: unknown node kind for %q", parent)
	}
	var folders []metadata.Folder
	for _, folder := range node.Folders {
		if child, ok := tree.Node(folder.Child); ok && child.Scope {
			folders = append(folders, folder)
		}
	}
	if len(folders) == 0 {
		return nil, "", nil
	}
	database := tree.DatabaseOf(parent)
	var scopes []metadata.ScopePath
	var current metadata.ScopePath
	for _, folder := range folders {
		children, err := live.LoadChildren(ctx, database, folder, []metadata.ScopePath{parent})
		if err != nil {
			return nil, "", err
		}
		for _, child := range children[parent] {
			if child.System {
				continue
			}
			path := parent.Child(metadata.ScopeSegment{Kind: child.Kind, Name: child.Name})
			scopes = append(scopes, path)
			if child.Current && current == "" {
				current = path
			}
		}
	}
	return scopes, current, nil
}
