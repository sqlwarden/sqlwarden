package completioncore

import (
	"slices"
	"sort"
	"strings"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

const (
	demandExplicit = iota
	demandReferenced
	demandDefault
)

type rankedDemand struct {
	demand   metadata.Demand
	priority int
}

// SchemaResolver reads a CompletionView and records a Demand for every
// folder a lookup needed but the view had not listed. It never fetches.
type SchemaResolver struct {
	view          *metadata.CompletionView
	defaultSchema string
	demands       []rankedDemand
	seen          map[metadata.Demand]int
}

func NewSchemaResolver(view *metadata.CompletionView, defaultSchema string) *SchemaResolver {
	if view == nil {
		view = metadata.NewCompletionView(metadata.Tree{}, "", "", nil, nil)
	}
	return &SchemaResolver{view: view, defaultSchema: defaultSchema, seen: map[metadata.Demand]int{}}
}

func (r *SchemaResolver) record(priority int, demands ...metadata.Demand) {
	for _, demand := range demands {
		if i, ok := r.seen[demand]; ok {
			r.demands[i].priority = min(r.demands[i].priority, priority)
			continue
		}
		r.seen[demand] = len(r.demands)
		r.demands = append(r.demands, rankedDemand{demand: demand, priority: priority})
	}
}

// Demands returns recorded demands, explicit qualifiers first, then
// statement references, then the default scope; ties keep first-seen order.
func (r *SchemaResolver) Demands() []metadata.Demand {
	ranked := slices.Clone(r.demands)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].priority < ranked[j].priority })
	out := make([]metadata.Demand, len(ranked))
	for i, item := range ranked {
		out[i] = item.demand
	}
	return out
}

func (r *SchemaResolver) priorityFor(database, namespace string) int {
	if database != "" || (namespace != "" && !strings.EqualFold(namespace, r.defaultSchema)) {
		return demandExplicit
	}
	return demandDefault
}

func (r *SchemaResolver) DefaultDatabase() string { return r.view.DefaultScope().Name("database") }
func (r *SchemaResolver) DefaultSchema() string   { return r.defaultSchema }

func (r *SchemaResolver) DatabaseNames() []string {
	return scopeNames(r.scopes("", demandDefault), "database")
}

func (r *SchemaResolver) SchemaNames(database string) []string {
	parent, ok := r.databaseScope(database, demandExplicit)
	if !ok {
		return nil
	}
	if len(r.view.ScopeFolders(parent)) == 0 {
		parent = ""
	}
	return scopeNames(r.scopes(parent, demandDefault), "schema")
}

func (r *SchemaResolver) Relations(database, namespace string) []Relation {
	if database == "" && namespace == "" {
		if scopes := r.searchScopes(demandDefault); scopes != nil {
			return r.relations(r.searchObjects(scopes, demandDefault, relationKinds...))
		}
	}
	priority := r.priorityFor(database, namespace)
	scope, ok := r.resolveScope(database, namespace, priority)
	if !ok {
		if database == "" && namespace == "" {
			r.record(demandDefault, r.view.ScopeDemands("")...)
			return r.allRelations()
		}
		return nil
	}
	refs, loaded := r.view.Objects(scope, relationKinds...)
	if !loaded {
		r.record(priority, r.view.ObjectDemands(scope, relationKinds...)...)
	}
	return r.relations(refs)
}

func (r *SchemaResolver) FindRelation(database, namespace, name string) (Relation, bool) {
	if database == "" && namespace == "" {
		if scopes := r.searchScopes(demandReferenced); scopes != nil {
			return r.findInScopes(scopes, name)
		}
	}
	namespaces := []string{namespace}
	if namespace == "" && r.defaultSchema != "" {
		namespaces = append(namespaces, r.defaultSchema)
	}
	for _, candidate := range namespaces {
		scope, ok := r.resolveScope(database, candidate, demandReferenced)
		if !ok {
			continue
		}
		refs, loaded := r.view.Objects(scope, relationKinds...)
		if ref, ok := matchRef(refs, name); ok {
			if _, columnsLoaded := r.view.Columns(ref); !columnsLoaded {
				r.record(demandReferenced, r.view.ColumnDemands(ref)...)
			}
			return r.relation(ref)
		}
		if !loaded {
			r.record(demandReferenced, r.view.ObjectDemands(scope, relationKinds...)...)
		}
	}
	return Relation{}, false
}

func (r *SchemaResolver) CatalogObjects(database, namespace string, kinds ...string) []metadata.ObjectRef {
	if database == "" && namespace == "" {
		if scopes := r.searchScopes(demandDefault); scopes != nil {
			return r.searchObjects(scopes, demandDefault, kinds...)
		}
	}
	if namespace == "" {
		namespace = r.defaultSchema
	}
	priority := r.priorityFor(database, namespace)
	scope, ok := r.resolveScope(database, namespace, priority)
	if !ok {
		return nil
	}
	refs, loaded := r.view.Objects(scope, kinds...)
	if !loaded {
		r.record(priority, r.view.ObjectDemands(scope, kinds...)...)
	}
	return refs
}

// Child returns ref's listing entry with its loader attributes.
func (r *SchemaResolver) Child(ref metadata.ObjectRef) (metadata.Child, bool) {
	return r.view.Child(ref)
}

// searchScopes resolves the view's search scopes when the tree declares
// fallbacks, and returns nil otherwise so single-scope lookups keep their
// resolveScope path. Fallbacks resolve through the parent's scope listing: a
// listed parent without the fallback drops it.
func (r *SchemaResolver) searchScopes(priority int) []metadata.ScopePath {
	paths := r.view.SearchScopes()
	if len(paths) < 2 {
		return nil
	}
	out := []metadata.ScopePath{paths[0]}
	for _, path := range paths[1:] {
		last, _ := path.Last()
		scope, ok := r.childScope(path.Parent(), last.Name, priority)
		if ok && !slices.Contains(out, scope) {
			out = append(out, scope)
		}
	}
	return out
}

// searchObjects lists kinds across scopes in search order. A name found in an
// earlier scope hides the same name, compared case-insensitively, in later
// scopes.
func (r *SchemaResolver) searchObjects(scopes []metadata.ScopePath, priority int, kinds ...string) []metadata.ObjectRef {
	hidden := map[string]bool{}
	var out []metadata.ObjectRef
	for _, scope := range scopes {
		refs, loaded := r.view.Objects(scope, kinds...)
		if !loaded {
			r.record(priority, r.view.ObjectDemands(scope, kinds...)...)
		}
		var names []string
		for _, ref := range refs {
			name := strings.ToLower(ref.Name)
			if hidden[name] {
				continue
			}
			out = append(out, ref)
			names = append(names, name)
		}
		for _, name := range names {
			hidden[name] = true
		}
	}
	return out
}

// findInScopes resolves name in search order. Once a scope is unlisted, a
// match in a later scope cannot be trusted because the unlisted scope may
// shadow it; later unlisted scopes are still demanded so one round loads
// them all.
func (r *SchemaResolver) findInScopes(scopes []metadata.ScopePath, name string) (Relation, bool) {
	blocked := false
	for _, scope := range scopes {
		refs, loaded := r.view.Objects(scope, relationKinds...)
		if !blocked {
			if ref, ok := matchRef(refs, name); ok {
				if _, columnsLoaded := r.view.Columns(ref); !columnsLoaded {
					r.record(demandReferenced, r.view.ColumnDemands(ref)...)
				}
				return r.relation(ref)
			}
		}
		if !loaded {
			r.record(demandReferenced, r.view.ObjectDemands(scope, relationKinds...)...)
			blocked = true
		}
	}
	return Relation{}, false
}

// resolveScope maps a (database, namespace) pair onto a scope path using
// only the navigator grammar: nested trees (database > schema) resolve the
// namespace under the database; flat trees (a single scope level) accept the
// namespace as the root scope name. When the default scope cannot be
// completed because the listing that would mark the current scope is
// missing, that listing is demanded.
func (r *SchemaResolver) resolveScope(database, namespace string, priority int) (metadata.ScopePath, bool) {
	base, ok := r.databaseScope(database, priority)
	if !ok {
		return "", false
	}
	nested := func(path metadata.ScopePath) bool { return len(r.view.ScopeFolders(path)) > 0 }
	if namespace == "" {
		defaultScope := r.view.DefaultScope()
		if defaultScope != "" && defaultScope.Within(base) {
			if nested(defaultScope) {
				r.scopes(defaultScope, demandDefault)
			}
			return defaultScope, true
		}
		if base == "" {
			r.scopes("", priority)
			return "", false
		}
		if !nested(base) {
			return base, true
		}
		return "", false
	}
	if base != "" && nested(base) {
		if scope, ok := r.childScope(base, namespace, priority); ok {
			return scope, true
		}
	}
	if database == "" && r.flatRoot() {
		if scope, ok := r.childScope("", namespace, priority); ok {
			return scope, true
		}
	}
	if base == "" && !r.flatRoot() {
		roots := r.scopes("", priority)
		for _, root := range roots {
			scopes, _ := r.view.Scopes(root)
			if scope, ok := matchScope(scopes, namespace); ok {
				return scope, true
			}
		}
	}
	return "", false
}

// databaseScope resolves a database qualifier to its root-level scope. The
// default base is known to exist, so naming it needs no root listing.
func (r *SchemaResolver) databaseScope(database string, priority int) (metadata.ScopePath, bool) {
	base := r.defaultBase()
	if database == "" {
		return base, true
	}
	if last, ok := base.Last(); ok && strings.EqualFold(last.Name, database) {
		return base, true
	}
	return r.childScope("", database, priority)
}

// defaultBase is the root-level scope completion falls back to: the default
// scope's first segment, else the only listed root scope.
func (r *SchemaResolver) defaultBase() metadata.ScopePath {
	if segments, err := r.view.DefaultScope().Segments(); err == nil && len(segments) > 0 {
		return metadata.NewScopePath(segments[0])
	}
	if roots, _ := r.view.Scopes(""); len(roots) == 1 {
		return roots[0]
	}
	return ""
}

// flatRoot reports whether root-level scopes hold no further scopes. It reads
// node kinds from the grammar directly: a ScopePath segment with an empty
// name does not decode, so a synthetic path would resolve to the root node.
func (r *SchemaResolver) flatRoot() bool {
	tree := r.view.Tree()
	for _, folder := range r.view.ScopeFolders("") {
		node, _ := tree.Node(folder.Child)
		for _, child := range node.Folders {
			if childNode, ok := tree.Node(child.Child); ok && childNode.Scope {
				return false
			}
		}
	}
	return true
}

func (r *SchemaResolver) scopes(parent metadata.ScopePath, priority int) []metadata.ScopePath {
	scopes, loaded := r.view.Scopes(parent)
	if !loaded {
		r.record(priority, r.view.ScopeDemands(parent)...)
	}
	return scopes
}

// childScope finds a listed child scope by name. When the parent's scope
// folder is unlisted it records the demand and, if the grammar allows only one
// scope kind there, returns a speculative path so dependent folders can be
// demanded in the same round.
func (r *SchemaResolver) childScope(parent metadata.ScopePath, name string, priority int) (metadata.ScopePath, bool) {
	scopes, loaded := r.view.Scopes(parent)
	if scope, ok := matchScope(scopes, name); ok {
		return scope, true
	}
	if loaded {
		return "", false
	}
	r.record(priority, r.view.ScopeDemands(parent)...)
	folders := r.view.ScopeFolders(parent)
	if len(folders) != 1 {
		return "", false
	}
	return parent.Child(metadata.ScopeSegment{Kind: folders[0].Child, Name: name}), true
}

func (r *SchemaResolver) relations(refs []metadata.ObjectRef) []Relation {
	out := make([]Relation, 0, len(refs))
	for _, ref := range refs {
		if relation, ok := r.relation(ref); ok {
			out = append(out, relation)
		}
	}
	return out
}

func (r *SchemaResolver) allRelations() []Relation {
	var refs []metadata.ObjectRef
	for _, ref := range r.view.Refs() {
		if slices.Contains(relationKinds, ref.Kind) {
			refs = append(refs, ref)
		}
	}
	return r.relations(refs)
}

func (r *SchemaResolver) relation(ref metadata.ObjectRef) (Relation, bool) {
	object, ok := r.view.Object(ref)
	if !ok {
		object = metadata.Object{Ref: ref}
	}
	if object.Relational == nil {
		if columns, _ := r.view.Columns(ref); len(columns) > 0 {
			object.Relational = &metadata.RelationalDetail{Columns: columns}
		}
	}
	return relationFromObject(object)
}

func matchScope(scopes []metadata.ScopePath, name string) (metadata.ScopePath, bool) {
	for _, scope := range scopes {
		if last, _ := scope.Last(); last.Name == name {
			return scope, true
		}
	}
	for _, scope := range scopes {
		if last, _ := scope.Last(); strings.EqualFold(last.Name, name) {
			return scope, true
		}
	}
	return "", false
}

func matchRef(refs []metadata.ObjectRef, name string) (metadata.ObjectRef, bool) {
	for _, ref := range refs {
		if ref.Name == name {
			return ref, true
		}
	}
	for _, ref := range refs {
		if strings.EqualFold(ref.Name, name) {
			return ref, true
		}
	}
	return metadata.ObjectRef{}, false
}

func scopeNames(scopes []metadata.ScopePath, kind string) []string {
	var names []string
	for _, scope := range scopes {
		if last, _ := scope.Last(); last.Kind == kind {
			names = append(names, last.Name)
		}
	}
	sort.Strings(names)
	return slices.Compact(names)
}
