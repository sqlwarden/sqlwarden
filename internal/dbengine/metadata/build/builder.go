// Package build assembles the two-tier metadata models from the flat rows that a
// driver's inspection queries return: a DirectoryBuilder for the cheap listing,
// and a RelationalBuilder for typed object detail with qualified foreign keys.
// Neither is safe for concurrent use; each inspection uses its own builder.
package build

import "github.com/sqlwarden/internal/dbengine/metadata"

// DirectoryBuilder accumulates object refs per scope and emits them grouped by
// kind, in the order kinds were declared (undeclared kinds sort last, first-seen).
type DirectoryBuilder struct {
	kindOrder  []string
	kindSeen   map[string]bool
	scopeOrder []metadata.ScopePath
	scopes     map[metadata.ScopePath]*scopeGroup
}

type scopeGroup struct {
	path       metadata.ScopePath
	groupSeen  map[string]bool
	groupOrder []string
	groups     map[string][]metadata.ObjectRef
}

// NewDirectory returns an empty DirectoryBuilder.
func NewDirectory() *DirectoryBuilder {
	return &DirectoryBuilder{kindSeen: map[string]bool{}, scopes: map[metadata.ScopePath]*scopeGroup{}}
}

// DeclareKind fixes a kind's position in every scope's group ordering.
func (b *DirectoryBuilder) DeclareKind(kind string) {
	if !b.kindSeen[kind] {
		b.kindSeen[kind] = true
		b.kindOrder = append(b.kindOrder, kind)
	}
}

// AddScope records an empty scope so hierarchy roots remain visible even when
// they contain objects only in child scopes.
func (b *DirectoryBuilder) AddScope(scope metadata.ScopePath) {
	if _, ok := b.scopes[scope]; ok {
		return
	}
	b.scopes[scope] = &scopeGroup{path: scope, groupSeen: map[string]bool{}, groups: map[string][]metadata.ObjectRef{}}
	b.scopeOrder = append(b.scopeOrder, scope)
}

// AddRef records an object of the given kind in the given scope.
func (b *DirectoryBuilder) AddRef(scope metadata.ScopePath, kind, name string) {
	b.AddScope(scope)
	n := b.scopes[scope]
	if !n.groupSeen[kind] {
		n.groupSeen[kind] = true
		n.groupOrder = append(n.groupOrder, kind)
	}
	n.groups[kind] = append(n.groups[kind], metadata.ObjectRef{Scope: scope, Kind: kind, Name: name})
}

// Build finalizes the directory with the given header fields. Scope nodes are
// assembled from their complete paths, so engines may expose arbitrary depth.
func (b *DirectoryBuilder) Build(connection, engine string, defaultScope metadata.ScopePath) *metadata.Directory {
	directory := &metadata.Directory{Connection: connection, Engine: engine, DefaultScope: defaultScope}
	nodes := make(map[metadata.ScopePath]*metadata.ScopeNode)
	roots := make([]metadata.ScopeNode, 0, len(b.scopeOrder))
	for _, scope := range b.scopeOrder {
		n := b.scopes[scope]
		node := metadata.ScopeNode{Path: scope, Groups: []metadata.ObjectGroup{}}
		emitted := map[string]bool{}
		emit := func(kind string) {
			refs := n.groups[kind]
			if len(refs) == 0 || emitted[kind] {
				return
			}
			emitted[kind] = true
			node.Groups = append(node.Groups, metadata.ObjectGroup{Kind: kind, Objects: refs})
		}
		for _, kind := range b.kindOrder {
			emit(kind)
		}
		for _, kind := range n.groupOrder { // undeclared kinds, first-seen
			emit(kind)
		}
		nodes[scope] = &node
	}
	for _, scope := range b.scopeOrder {
		node := nodes[scope]
		segments, _ := scope.Segments()
		if len(segments) <= 1 {
			roots = append(roots, *node)
			continue
		}
		parent := metadata.NewScopePath(segments[:len(segments)-1]...)
		if parentNode, ok := nodes[parent]; ok {
			parentNode.Children = append(parentNode.Children, *node)
		} else {
			roots = append(roots, *node)
		}
	}
	// Rebuild nodes after child attachment in deepest-first order.
	for i := len(b.scopeOrder) - 1; i >= 0; i-- {
		scope := b.scopeOrder[i]
		segments, _ := scope.Segments()
		if len(segments) <= 1 {
			continue
		}
		parent := metadata.NewScopePath(segments[:len(segments)-1]...)
		if parentNode, ok := nodes[parent]; ok {
			for childIndex := range parentNode.Children {
				if parentNode.Children[childIndex].Path == scope {
					parentNode.Children[childIndex] = *nodes[scope]
				}
			}
		}
	}
	for i := range roots {
		if current, ok := nodes[roots[i].Path]; ok {
			roots[i] = *current
		}
	}
	directory.Roots = roots
	return directory
}

// RelationalBuilder accumulates typed relational detail keyed by ObjectRef and
// emits objects in first-seen order, each carrying a Relational facet.
type RelationalBuilder struct {
	order   []metadata.ObjectRef
	objs    map[metadata.ObjectRef]*metadata.Object
	fkOrder map[metadata.ObjectRef][]string
	fks     map[metadata.ObjectRef]map[string]*metadata.ForeignKey
}

// NewRelational returns an empty RelationalBuilder.
func NewRelational() *RelationalBuilder {
	return &RelationalBuilder{
		objs:    map[metadata.ObjectRef]*metadata.Object{},
		fkOrder: map[metadata.ObjectRef][]string{},
		fks:     map[metadata.ObjectRef]map[string]*metadata.ForeignKey{},
	}
}

func (b *RelationalBuilder) object(ref metadata.ObjectRef) *metadata.Object {
	o, ok := b.objs[ref]
	if !ok {
		o = &metadata.Object{Ref: ref, Relational: &metadata.RelationalDetail{}}
		b.objs[ref] = o
		b.order = append(b.order, ref)
	}
	return o
}

// Ensure registers an object even if it has no columns yet.
func (b *RelationalBuilder) Ensure(ref metadata.ObjectRef) { b.object(ref) }

// AddColumn appends a column to the object's relational facet.
func (b *RelationalBuilder) AddColumn(ref metadata.ObjectRef, c metadata.Column) {
	o := b.object(ref)
	o.Relational.Columns = append(o.Relational.Columns, c)
}

// AddPrimaryKeyColumn appends a column to the object's primary key (call order).
func (b *RelationalBuilder) AddPrimaryKeyColumn(ref metadata.ObjectRef, col string) {
	o := b.object(ref)
	o.Relational.PrimaryKey = append(o.Relational.PrimaryKey, col)
}

// AddForeignKeyColumn appends a (column -> referenced column) pair to a named
// foreign key, creating it on first sight with the qualified target reference.
func (b *RelationalBuilder) AddForeignKeyColumn(ref metadata.ObjectRef, fkName, col string, references metadata.ObjectRef, refCol string) {
	b.object(ref)
	if b.fks[ref] == nil {
		b.fks[ref] = map[string]*metadata.ForeignKey{}
	}
	fk, ok := b.fks[ref][fkName]
	if !ok {
		fk = &metadata.ForeignKey{Name: fkName, References: references}
		b.fks[ref][fkName] = fk
		b.fkOrder[ref] = append(b.fkOrder[ref], fkName)
	}
	fk.Columns = append(fk.Columns, col)
	fk.ReferencedColumns = append(fk.ReferencedColumns, refCol)
}

// AddIndex appends an index to the object's relational facet.
func (b *RelationalBuilder) AddIndex(ref metadata.ObjectRef, ix metadata.SecondaryIndex) {
	o := b.object(ref)
	o.Relational.Indexes = append(o.Relational.Indexes, ix)
}

// Build attaches accumulated foreign keys and returns objects in first-seen order.
func (b *RelationalBuilder) Build() []metadata.Object {
	out := make([]metadata.Object, 0, len(b.order))
	for _, ref := range b.order {
		o := b.objs[ref]
		for _, fkName := range b.fkOrder[ref] {
			o.Relational.ForeignKeys = append(o.Relational.ForeignKeys, *b.fks[ref][fkName])
		}
		out = append(out, *o)
	}
	return out
}
