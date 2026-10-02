package metadata

import (
	"slices"
	"sort"
)

// Demand names a navigator folder whose listing completion needed but the
// cache did not hold.
type Demand struct {
	Parent ScopePath
	Folder string
}

type ListingKey struct {
	Parent ScopePath
	Folder string
}

// CompletionView is the immutable per-request metadata completion reads. A
// key present in listings with an empty slice is listed and empty; an absent
// key is unlisted.
type CompletionView struct {
	tree             Tree
	defaultScope     ScopePath
	effectiveDefault ScopePath
	listings         map[ListingKey][]Child
	objects          map[ObjectRef]Object
}

// NewCompletionView builds the view. sessionScope is the live session's
// current scope, or empty when unknown; it never comes from cached listings,
// whose current flags reflect whichever pooled connection ran the listing.
func NewCompletionView(tree Tree, defaultScope, sessionScope ScopePath, listings map[ListingKey][]Child, objects map[ObjectRef]Object) *CompletionView {
	if listings == nil {
		listings = map[ListingKey][]Child{}
	}
	if objects == nil {
		objects = map[ObjectRef]Object{}
	}
	v := &CompletionView{tree: tree, defaultScope: defaultScope, listings: listings, objects: objects}
	v.effectiveDefault = v.deriveDefaultScope(sessionScope)
	return v
}

// deriveDefaultScope extends the configured default scope with the session
// scope's deeper levels when the session scope lies within it. A session
// scope that disagrees with the configuration, or that names a non-scope
// level, leaves the configured scope as is.
func (v *CompletionView) deriveDefaultScope(session ScopePath) ScopePath {
	if session == "" || !session.Within(v.defaultScope) {
		return v.defaultScope
	}
	segments, err := session.Segments()
	if err != nil {
		return v.defaultScope
	}
	for _, segment := range segments {
		if node, ok := v.tree.Node(segment.Kind); !ok || !node.Scope {
			return v.defaultScope
		}
	}
	return session
}

func (v *CompletionView) Tree() Tree { return v.tree }

// DefaultScope is the scope unqualified names resolve against: the
// configured default scope, extended with the live session's current scope
// where the configuration stops short.
func (v *CompletionView) DefaultScope() ScopePath { return v.effectiveDefault }

// SearchScopes is the ordered list of scopes unqualified names resolve
// against: the default scope, then each of the tree's fallback scopes.
func (v *CompletionView) SearchScopes() []ScopePath {
	def := v.DefaultScope()
	if def == "" {
		return nil
	}
	out := []ScopePath{def}
	if len(v.tree.FallbackScopes) == 0 {
		return out
	}
	var base ScopePath
	var kind string
	switch folders := v.ScopeFolders(def); len(folders) {
	case 0:
		last, ok := def.Last()
		if !ok {
			return out
		}
		base, kind = def.Parent(), last.Kind
	case 1:
		base, kind = def, folders[0].Child
	default:
		return out
	}
	for _, name := range v.tree.FallbackScopes {
		path := base.Child(ScopeSegment{Kind: kind, Name: name})
		if !slices.Contains(out, path) {
			out = append(out, path)
		}
	}
	return out
}

func (v *CompletionView) Empty() bool { return len(v.listings) == 0 && len(v.objects) == 0 }

func (v *CompletionView) node(path ScopePath) (Node, bool) {
	return v.tree.Node(v.tree.NodeKindOf(path))
}

func (v *CompletionView) ScopeFolders(parent ScopePath) []Folder {
	node, ok := v.node(parent)
	if !ok {
		return nil
	}
	var out []Folder
	for _, folder := range node.Folders {
		if child, ok := v.tree.Node(folder.Child); ok && child.Scope {
			out = append(out, folder)
		}
	}
	return out
}

func (v *CompletionView) Scopes(parent ScopePath) ([]ScopePath, bool) {
	loaded := true
	var out []ScopePath
	for _, folder := range v.ScopeFolders(parent) {
		items, ok := v.listings[ListingKey{Parent: parent, Folder: folder.Kind}]
		if !ok {
			loaded = false
			continue
		}
		for _, item := range items {
			if node, ok := v.tree.Node(item.Kind); ok && node.Scope {
				out = append(out, parent.Child(ScopeSegment{Kind: item.Kind, Name: item.Name}))
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out), loaded
}

func (v *CompletionView) ScopeDemands(parent ScopePath) []Demand {
	return v.unlisted(parent, v.ScopeFolders(parent))
}

func (v *CompletionView) objectFolders(scope ScopePath, kinds []string) []Folder {
	node, ok := v.node(scope)
	if !ok {
		return nil
	}
	var out []Folder
	for _, folder := range node.Folders {
		if slices.ContainsFunc(kinds, folder.Contains) {
			out = append(out, folder)
		}
	}
	return out
}

func (v *CompletionView) Objects(scope ScopePath, kinds ...string) ([]ObjectRef, bool) {
	seen := map[ObjectRef]bool{}
	var out []ObjectRef
	add := func(ref ObjectRef) {
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	loaded := true
	for _, folder := range v.objectFolders(scope, kinds) {
		items, ok := v.listings[ListingKey{Parent: scope, Folder: folder.Kind}]
		if !ok {
			loaded = false
			continue
		}
		for _, item := range items {
			if slices.Contains(kinds, item.Kind) {
				add(ObjectRef{Scope: scope, Kind: item.Kind, Name: item.Name})
			}
		}
	}
	for ref := range v.objects {
		if ref.Scope == scope && slices.Contains(kinds, ref.Kind) {
			add(ref)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out, loaded
}

func (v *CompletionView) ObjectDemands(scope ScopePath, kinds ...string) []Demand {
	return v.unlisted(scope, v.objectFolders(scope, kinds))
}

func (v *CompletionView) Object(ref ObjectRef) (Object, bool) {
	object, ok := v.objects[ref]
	return object, ok
}

// Child returns ref's listing entry, which carries the loader's attributes.
func (v *CompletionView) Child(ref ObjectRef) (Child, bool) {
	for _, folder := range v.objectFolders(ref.Scope, []string{ref.Kind}) {
		for _, item := range v.listings[ListingKey{Parent: ref.Scope, Folder: folder.Kind}] {
			if item.Kind == ref.Kind && item.Name == ref.Name {
				return item, true
			}
		}
	}
	return Child{}, false
}

func (v *CompletionView) columnFolder(kind string) (Folder, bool) {
	node, ok := v.tree.Node(kind)
	if !ok {
		return Folder{}, false
	}
	for _, folder := range node.Folders {
		if child, ok := v.tree.Node(folder.Child); ok && child.Column {
			return folder, true
		}
	}
	return Folder{}, false
}

func (v *CompletionView) Columns(ref ObjectRef) ([]Column, bool) {
	if object, ok := v.objects[ref]; ok && object.Relational != nil {
		return object.Relational.Columns, true
	}
	folder, ok := v.columnFolder(ref.Kind)
	if !ok {
		return nil, true
	}
	items, ok := v.listings[ListingKey{Parent: ref.Path(), Folder: folder.Kind}]
	if !ok {
		return nil, false
	}
	columns := make([]Column, 0, len(items))
	for _, item := range items {
		if node, ok := v.tree.Node(item.Kind); ok && node.Column {
			columns = append(columns, columnFromChild(item))
		}
	}
	sort.SliceStable(columns, func(i, j int) bool { return columns[i].Ordinal < columns[j].Ordinal })
	return columns, true
}

func (v *CompletionView) ColumnDemands(ref ObjectRef) []Demand {
	folder, ok := v.columnFolder(ref.Kind)
	if !ok {
		return nil
	}
	return v.unlisted(ref.Path(), []Folder{folder})
}

func (v *CompletionView) unlisted(parent ScopePath, folders []Folder) []Demand {
	var out []Demand
	for _, folder := range folders {
		if _, ok := v.listings[ListingKey{Parent: parent, Folder: folder.Kind}]; !ok {
			out = append(out, Demand{Parent: parent, Folder: folder.Kind})
		}
	}
	return out
}

// ScopePaths returns the scopes known to exist: those listed as items, the
// default scope and its scope ancestors, and scopes of cached objects. A
// listing's parent alone does not count, since a speculative load under a
// mistyped or miscased qualifier caches an empty listing for a scope that
// does not exist.
func (v *CompletionView) ScopePaths() []ScopePath {
	var out []ScopePath
	if v.defaultScope != "" {
		for _, path := range append(v.defaultScope.Prefixes(), v.defaultScope) {
			if node, ok := v.node(path); ok && node.Scope && path != "" {
				out = append(out, path)
			}
		}
	}
	for key, items := range v.listings {
		for _, item := range items {
			if node, ok := v.tree.Node(item.Kind); ok && node.Scope {
				out = append(out, key.Parent.Child(ScopeSegment{Kind: item.Kind, Name: item.Name}))
			}
		}
	}
	for ref := range v.objects {
		if ref.Scope != "" {
			out = append(out, ref.Scope)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (v *CompletionView) Refs() []ObjectRef {
	seen := map[ObjectRef]bool{}
	var out []ObjectRef
	for key, items := range v.listings {
		parent, ok := v.node(key.Parent)
		if !ok || !parent.Scope {
			continue
		}
		for _, item := range items {
			node, ok := v.tree.Node(item.Kind)
			if !ok || node.Scope || node.Column {
				continue
			}
			ref := ObjectRef{Scope: key.Parent, Kind: item.Kind, Name: item.Name}
			if !seen[ref] {
				seen[ref] = true
				out = append(out, ref)
			}
		}
	}
	for ref := range v.objects {
		if !seen[ref] {
			seen[ref] = true
			out = append(out, ref)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out
}

func columnFromChild(item Child) Column {
	column := Column{Name: item.Name}
	if value, ok := item.Attributes["data_type"].(string); ok {
		column.DataType = value
	}
	if value, ok := item.Attributes["nullable"].(bool); ok {
		column.Nullable = value
	}
	column.Ordinal = intAttribute(item.Attributes["ordinal"])
	return column
}

// intAttribute accepts the integer types loaders produce and the float64 a
// JSON round trip through the store yields.
func intAttribute(value any) int {
	switch n := value.(type) {
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
