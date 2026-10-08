package schema

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"time"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

// CompletionView projects whatever the navigator has cached for conn into the
// per-request completion input. Listings pass through the same read-time
// presentation as the tree, and subtrees of items it hides are pruned. The
// live session, when present, supplies the current scope that fills in the
// levels the configured default scope leaves open.
func (n *Navigator) CompletionView(ctx context.Context, conn Connection, tree metadata.Tree, live Live) (*metadata.CompletionView, error) {
	read := n.decoded.begin(conn.ID)
	listings, err := n.cachedWithin(ctx, conn, "", read)
	if err != nil {
		return nil, err
	}
	objects, err := n.cachedObjects(ctx, conn, read)
	if err != nil {
		return nil, err
	}
	read.commit()
	keys := make([]listingKey, 0, len(listings))
	for key := range listings {
		keys = append(keys, key)
	}
	// Parent paths sort before their descendants, so hidden subtrees are
	// recorded before their own listings are visited.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].parent != keys[j].parent {
			return keys[i].parent < keys[j].parent
		}
		return keys[i].folder < keys[j].folder
	})
	visible := make(map[metadata.ListingKey][]metadata.Child, len(keys))
	var hidden []metadata.ScopePath
	for _, key := range keys {
		if withinAny(key.parent, hidden) {
			continue
		}
		raw := listings[key]
		shown := n.present(conn, tree, Listing{Parent: key.parent, Folder: key.folder, Items: raw}).Items
		hidden = append(hidden, removedPaths(key.parent, raw, shown)...)
		visible[metadata.ListingKey{Parent: key.parent, Folder: key.folder}] = shown
	}
	if conn.ShowAllDatabases {
		addCachedDatabases(tree, visible, objects, hidden)
	} else {
		n.presentUnlistedDatabases(conn, tree, visible)
	}
	databases := listedDatabases(tree, visible)
	for key := range visible {
		if !databases.allows(tree, key.Parent) {
			delete(visible, key)
		}
	}
	for ref := range objects {
		if withinAny(ref.Path(), hidden) || !databases.allows(tree, ref.Path()) {
			delete(objects, ref)
		}
	}
	return metadata.NewCompletionView(tree, conn.DefaultScope, n.sessionScope(ctx, conn.ID, live), visible, objects), nil
}

const (
	sessionScopeTTL     = 30 * time.Second
	sessionScopeTimeout = 2 * time.Second
)

type sessionScopeMemo struct {
	scope metadata.ScopePath
	at    time.Time
}

// sessionScope returns the live session's current scope, memoized per
// connection. A failed lookup yields no fill-in and is memoized like a
// result, so a session that cannot report its scope is not asked on every
// keystroke.
func (n *Navigator) sessionScope(ctx context.Context, connID int64, live Live) metadata.ScopePath {
	if live == nil {
		return ""
	}
	now := n.now()
	n.mu.Lock()
	memo, ok := n.sessionScopes[connID]
	n.mu.Unlock()
	if ok && now.Sub(memo.at) < sessionScopeTTL {
		return memo.scope
	}
	lookupCtx, cancel := context.WithTimeout(ctx, sessionScopeTimeout)
	defer cancel()
	scope, err := live.CurrentScope(lookupCtx)
	if err != nil {
		if ctx.Err() != nil {
			return ""
		}
		n.logger.Debug("session scope unavailable", slog.Int64("connection_id", connID), slog.Any("error", err))
		scope = ""
	}
	n.mu.Lock()
	n.sessionScopes[connID] = sessionScopeMemo{scope: scope, at: now}
	n.mu.Unlock()
	return scope
}

// presentUnlistedDatabases fills an uncached root database folder with what
// the tree would show for it when only the default database is visible, so
// completion neither offers nor demands the other databases.
func (n *Navigator) presentUnlistedDatabases(conn Connection, tree metadata.Tree, visible map[metadata.ListingKey][]metadata.Child) {
	for _, folder := range databaseFolders(tree) {
		key := metadata.ListingKey{Parent: "", Folder: folder.Kind}
		if _, ok := visible[key]; ok {
			continue
		}
		if items := n.present(conn, tree, Listing{Folder: folder.Kind}).Items; len(items) > 0 {
			visible[key] = items
		}
	}
}

// addCachedDatabases adds to a listed root database folder the databases it
// omits but whose own contents are cached, such as one created after the root
// was listed. Only non-empty listings or cached objects count: a speculative
// load under a mistyped qualifier caches an empty listing for a database that
// does not exist.
func addCachedDatabases(tree metadata.Tree, visible map[metadata.ListingKey][]metadata.Child, objects map[metadata.ObjectRef]metadata.Object, hidden []metadata.ScopePath) {
	kind := tree.DatabaseKind()
	folders := databaseFolders(tree)
	if len(folders) == 0 {
		return
	}
	root := metadata.ListingKey{Parent: "", Folder: folders[0].Kind}
	if _, ok := visible[root]; !ok {
		return
	}
	listed := listedDatabases(tree, visible)
	cached := map[string]bool{}
	for key, items := range visible {
		if len(items) > 0 {
			cached[tree.DatabaseOf(key.Parent)] = true
		}
	}
	for ref := range objects {
		if !withinAny(ref.Path(), hidden) {
			cached[tree.DatabaseOf(ref.Path())] = true
		}
	}
	var added []string
	for database := range cached {
		if database != "" && !listed[database] {
			added = append(added, database)
		}
	}
	sort.Strings(added)
	items := slices.Clone(visible[root])
	for _, database := range added {
		items = append(items, metadata.Child{Kind: kind, Name: database})
	}
	visible[root] = items
}

func databaseFolders(tree metadata.Tree) []metadata.Folder {
	kind := tree.DatabaseKind()
	if kind == "" {
		return nil
	}
	var out []metadata.Folder
	for _, folder := range tree.Root.Folders {
		if folder.Child == kind {
			out = append(out, folder)
		}
	}
	return out
}

// databaseSet holds the databases visible in the root database folder, or is
// nil when that folder is not listed and every database is allowed.
type databaseSet map[string]bool

func listedDatabases(tree metadata.Tree, visible map[metadata.ListingKey][]metadata.Child) databaseSet {
	var out databaseSet
	for _, folder := range databaseFolders(tree) {
		items, ok := visible[metadata.ListingKey{Parent: "", Folder: folder.Kind}]
		if !ok {
			continue
		}
		if out == nil {
			out = databaseSet{}
		}
		for _, item := range items {
			out[item.Name] = true
		}
	}
	return out
}

func (s databaseSet) allows(tree metadata.Tree, path metadata.ScopePath) bool {
	database := tree.DatabaseOf(path)
	return s == nil || database == "" || s[database]
}

func removedPaths(parent metadata.ScopePath, raw, shown []metadata.Child) []metadata.ScopePath {
	kept := make(map[metadata.ScopeSegment]bool, len(shown))
	for _, item := range shown {
		kept[metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}] = true
	}
	var out []metadata.ScopePath
	for _, item := range raw {
		segment := metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}
		if !kept[segment] {
			out = append(out, parent.Child(segment))
		}
	}
	return out
}

func (n *Navigator) cachedObjects(ctx context.Context, conn Connection, read *memoRead) (map[metadata.ObjectRef]metadata.Object, error) {
	out := map[metadata.ObjectRef]metadata.Object{}
	if conn.Persistent && n.store != nil {
		rows, err := n.store.AllSchemaObjects(ctx, conn.ID)
		if err != nil {
			return nil, fmt.Errorf("read cached objects: %w", err)
		}
		for _, row := range rows {
			obj, err := read.object(row)
			if err != nil {
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
