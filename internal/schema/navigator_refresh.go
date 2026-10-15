package schema

import (
	"context"
	"fmt"
	"sort"

	"github.com/sqlwarden/internal/database"
	metadata "github.com/sqlwarden/internal/engine/metadata"
)

type refreshGroup struct {
	database   string
	depth      int
	parentKind string
	folder     metadata.Folder
	parents    []metadata.ScopePath
}

func (n *Navigator) Refresh(ctx context.Context, conn Connection, tree metadata.Tree, live Live, root metadata.ScopePath) ([]Listing, error) {
	if live == nil {
		return nil, ErrSessionRequired
	}
	previous, err := n.cachedWithin(ctx, conn, root, nil)
	if err != nil {
		return nil, err
	}
	if root != "" {
		key, items, ok, err := n.containingListing(ctx, conn, tree, root)
		if err != nil {
			return nil, err
		}
		if ok {
			previous[key] = items
		}
	}

	var removed []metadata.ScopePath
	var refreshed []Listing
	groups := groupRefreshKeys(tree, previous)
	for _, group := range groups {
		parents := make([]metadata.ScopePath, 0, len(group.parents))
		for _, parent := range group.parents {
			if !withinAny(parent, removed) {
				parents = append(parents, parent)
			}
		}
		if len(parents) == 0 {
			continue
		}
		listings, err := n.load(ctx, live, group.database, group.folder, parents)
		if err != nil {
			return nil, fmt.Errorf("refresh %s: %w", group.folder.Kind, err)
		}
		for _, listing := range listings {
			key := listingKey{parent: listing.Parent, folder: listing.Folder}
			removed = append(removed, vanishedPaths(listing, previous[key])...)
		}
		refreshed = append(refreshed, listings...)
	}

	for _, path := range removed {
		if err := n.forgetWithin(ctx, conn, path); err != nil {
			return nil, err
		}
	}
	if err := n.save(ctx, conn, refreshed); err != nil {
		return nil, err
	}
	if err := n.invalidateDetail(ctx, conn, root); err != nil {
		return nil, err
	}
	n.logger.Debug("schema navigator refreshed", "connection_id", conn.ID, "groups", len(groups), "listings", len(refreshed), "removed_subtrees", len(removed))

	out := make([]Listing, 0, len(refreshed))
	for _, listing := range refreshed {
		listing.Source = SourceLive
		out = append(out, n.present(conn, tree, listing))
	}
	return out, nil
}

// cachedWithin returns every cached listing whose parent is within root,
// memory taking precedence over the store.
func (n *Navigator) cachedWithin(ctx context.Context, conn Connection, root metadata.ScopePath, read *memoRead) (map[listingKey][]metadata.Child, error) {
	out := map[listingKey][]metadata.Child{}
	if conn.Persistent && n.store != nil {
		rows, err := n.store.SchemaListingsWithin(ctx, conn.ID, string(root))
		if err != nil {
			return nil, fmt.Errorf("read cached listings: %w", err)
		}
		for _, row := range rows {
			listing, err := read.listing(row)
			if err != nil {
				return nil, err
			}
			out[listingKey{parent: listing.Parent, folder: listing.Folder}] = listing.Items
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	for key, listing := range n.memoryFor(conn.ID).listings {
		if key.parent.Within(root) {
			out[key] = listing.Items
		}
	}
	return out, nil
}

// containingListing resolves the parent listing root was listed in. Several
// folders under one parent may share a child kind, so the cached listing that
// holds root wins; without one, the first declared folder is assumed.
func (n *Navigator) containingListing(ctx context.Context, conn Connection, tree metadata.Tree, root metadata.ScopePath) (listingKey, []metadata.Child, bool, error) {
	parent := root.Parent()
	segment, ok := root.Last()
	if !ok {
		return listingKey{}, nil, false, nil
	}
	folders := tree.FoldersContaining(tree.NodeKindOf(parent), segment.Kind)
	if len(folders) == 0 {
		return listingKey{}, nil, false, nil
	}
	var fallback []metadata.Child
	for i, folder := range folders {
		key := listingKey{parent: parent, folder: folder.Kind}
		items, err := n.cachedListingItems(ctx, conn, key)
		if err != nil {
			return listingKey{}, nil, false, err
		}
		if i == 0 {
			fallback = items
		}
		for _, item := range items {
			if item.Kind == segment.Kind && item.Name == segment.Name {
				return key, items, true, nil
			}
		}
	}
	return listingKey{parent: parent, folder: folders[0].Kind}, fallback, true, nil
}

func (n *Navigator) cachedListingItems(ctx context.Context, conn Connection, key listingKey) ([]metadata.Child, error) {
	n.mu.Lock()
	listing, ok := n.memoryFor(conn.ID).listings[key]
	n.mu.Unlock()
	if ok {
		return listing.Items, nil
	}
	if !conn.Persistent || n.store == nil {
		return nil, nil
	}
	row, found, err := n.store.SchemaListing(ctx, conn.ID, string(key.parent), key.folder)
	if err != nil {
		return nil, fmt.Errorf("read cached listing: %w", err)
	}
	if !found {
		return nil, nil
	}
	listing, err = decodeListing(row)
	if err != nil {
		return nil, err
	}
	return listing.Items, nil
}

func groupRefreshKeys(tree metadata.Tree, keys map[listingKey][]metadata.Child) []refreshGroup {
	type groupID struct {
		database   string
		depth      int
		parentKind string
		folder     string
	}
	groups := map[groupID]*refreshGroup{}
	for key := range keys {
		parentKind := tree.NodeKindOf(key.parent)
		folder, ok := tree.Folder(parentKind, key.folder)
		if !ok {
			continue
		}
		id := groupID{database: tree.DatabaseOf(key.parent), depth: key.parent.Depth(), parentKind: parentKind, folder: key.folder}
		group, ok := groups[id]
		if !ok {
			group = &refreshGroup{database: id.database, depth: id.depth, parentKind: parentKind, folder: folder}
			groups[id] = group
		}
		group.parents = append(group.parents, key.parent)
	}
	out := make([]refreshGroup, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group.parents, func(i, j int) bool { return group.parents[i] < group.parents[j] })
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		if a.database != b.database {
			return a.database < b.database
		}
		if a.parentKind != b.parentKind {
			return a.parentKind < b.parentKind
		}
		return a.folder.Kind < b.folder.Kind
	})
	return out
}

func vanishedPaths(listing Listing, previous []metadata.Child) []metadata.ScopePath {
	current := make(map[metadata.ScopeSegment]bool, len(listing.Items))
	for _, item := range listing.Items {
		current[metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}] = true
	}
	var out []metadata.ScopePath
	for _, item := range previous {
		segment := metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}
		if !current[segment] {
			out = append(out, listing.Parent.Child(segment))
		}
	}
	return out
}

func withinAny(path metadata.ScopePath, roots []metadata.ScopePath) bool {
	for _, root := range roots {
		if path.Within(root) {
			return true
		}
	}
	return false
}

func (n *Navigator) forgetWithin(ctx context.Context, conn Connection, root metadata.ScopePath) error {
	n.mu.Lock()
	mem := n.memoryFor(conn.ID)
	for key := range mem.listings {
		if key.parent.Within(root) {
			delete(mem.listings, key)
		}
	}
	n.mu.Unlock()
	if conn.Persistent && n.store != nil {
		if err := n.store.DeleteSchemaListingsWithin(ctx, conn.ID, string(root)); err != nil {
			return fmt.Errorf("delete cached listings: %w", err)
		}
	}
	return n.invalidateDetail(ctx, conn, root)
}

func (n *Navigator) invalidateDetail(ctx context.Context, conn Connection, root metadata.ScopePath) error {
	ancestors := append(root.Prefixes(), root)
	n.mu.Lock()
	mem := n.memoryFor(conn.ID)
	for ref := range mem.objects {
		if ref.Path().Within(root) {
			delete(mem.objects, ref)
		}
	}
	for scope := range mem.relationships {
		if scope.Within(root) || containsPath(ancestors, scope) {
			delete(mem.relationships, scope)
		}
	}
	n.mu.Unlock()
	if !conn.Persistent || n.store == nil {
		return nil
	}
	var exact []database.SchemaObjectKey
	if ref, ok := metadata.ObjectRefOf(root); ok {
		exact = append(exact, database.SchemaObjectKey{Scope: string(ref.Scope), Kind: ref.Kind, Name: ref.Name})
	}
	if err := n.store.DeleteSchemaObjects(ctx, conn.ID, string(root), exact); err != nil {
		return fmt.Errorf("delete cached objects: %w", err)
	}
	scopes := make([]string, 0, len(ancestors))
	for _, scope := range ancestors {
		scopes = append(scopes, string(scope))
	}
	if err := n.store.DeleteSchemaRelationships(ctx, conn.ID, string(root), scopes); err != nil {
		return fmt.Errorf("delete cached relationships: %w", err)
	}
	return nil
}

func containsPath(paths []metadata.ScopePath, path metadata.ScopePath) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}
	return false
}
