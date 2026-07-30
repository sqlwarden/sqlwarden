// Package build assembles the two-tier schema models from the flat rows that a
// driver's inspection queries return: a CatalogBuilder for the cheap listing,
// and a RelationalBuilder for typed object detail with qualified foreign keys.
// Neither is safe for concurrent use; each inspection uses its own builder.
package build

import "github.com/sqlwarden/internal/dbengine/schema"

// CatalogBuilder accumulates object refs per namespace and emits them grouped by
// kind, in the order kinds were declared (undeclared kinds sort last, first-seen).
type CatalogBuilder struct {
	kindOrder  []string
	kindSeen   map[string]bool
	scopeOrder []schema.ScopePath
	scopes     map[schema.ScopePath]*nsCat
}

type nsCat struct {
	path       schema.ScopePath
	groupSeen  map[string]bool
	groupOrder []string
	groups     map[string][]schema.ObjectRef
}

// NewCatalog returns an empty CatalogBuilder.
func NewCatalog() *CatalogBuilder {
	return &CatalogBuilder{kindSeen: map[string]bool{}, scopes: map[schema.ScopePath]*nsCat{}}
}

// DeclareKind fixes a kind's position in every namespace's group ordering.
func (b *CatalogBuilder) DeclareKind(kind string) {
	if !b.kindSeen[kind] {
		b.kindSeen[kind] = true
		b.kindOrder = append(b.kindOrder, kind)
	}
}

// AddRef records an object of the given kind in the given scope.
func (b *CatalogBuilder) AddRef(rawScope any, kind, name string) {
	var scope schema.ScopePath
	switch value := rawScope.(type) {
	case schema.ScopePath:
		scope = value
	case string:
		scope = schema.NewScopePath(schema.ScopeSegment{Kind: "schema", Name: value})
	default:
		return
	}
	n, ok := b.scopes[scope]
	if !ok {
		n = &nsCat{path: scope, groupSeen: map[string]bool{}, groups: map[string][]schema.ObjectRef{}}
		b.scopes[scope] = n
		b.scopeOrder = append(b.scopeOrder, scope)
	}
	if !n.groupSeen[kind] {
		n.groupSeen[kind] = true
		n.groupOrder = append(n.groupOrder, kind)
	}
	n.groups[kind] = append(n.groups[kind], schema.ObjectRef{Scope: scope, Kind: kind, Name: name})
}

// Build retains the current one-database catalog contract during migration.
func (b *CatalogBuilder) Build(connection, engine, database string) *schema.Catalog {
	catalog := &schema.Catalog{Connection: connection, Dialect: engine, Database: database}
	for _, scope := range b.scopeOrder {
		n := b.scopes[scope]
		segments, _ := scope.Segments()
		name := ""
		if len(segments) > 0 {
			name = segments[len(segments)-1].Name
		}
		namespace := schema.NamespaceCatalog{Name: name}
		emitted := map[string]bool{}
		emit := func(kind string) {
			refs := n.groups[kind]
			if len(refs) == 0 || emitted[kind] {
				return
			}
			emitted[kind] = true
			for index := range refs {
				refs[index].Namespace = name
				refs[index].Scope = ""
			}
			namespace.Groups = append(namespace.Groups, schema.ObjectGroupCatalog{Kind: kind, Objects: refs})
		}
		for _, kind := range b.kindOrder {
			emit(kind)
		}
		for _, kind := range n.groupOrder {
			emit(kind)
		}
		catalog.Namespaces = append(catalog.Namespaces, namespace)
	}
	return catalog
}

// BuildDirectory finalizes the directory with the given header fields. Scope nodes are
// assembled from their complete paths, so engines may expose arbitrary depth.
func (b *CatalogBuilder) BuildDirectory(connection, engine string, defaultScope schema.ScopePath) *schema.Directory {
	directory := &schema.Directory{Connection: connection, Engine: engine, DefaultScope: defaultScope}
	nodes := make(map[schema.ScopePath]*schema.ScopeNode)
	var roots []schema.ScopeNode
	for _, scope := range b.scopeOrder {
		n := b.scopes[scope]
		node := schema.ScopeNode{Path: scope}
		emitted := map[string]bool{}
		emit := func(kind string) {
			refs := n.groups[kind]
			if len(refs) == 0 || emitted[kind] {
				return
			}
			emitted[kind] = true
			node.Groups = append(node.Groups, schema.ObjectGroupCatalog{Kind: kind, Objects: refs})
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
		parent := schema.NewScopePath(segments[:len(segments)-1]...)
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
		parent := schema.NewScopePath(segments[:len(segments)-1]...)
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
	order   []schema.ObjectRef
	objs    map[schema.ObjectRef]*schema.Object
	fkOrder map[schema.ObjectRef][]string
	fks     map[schema.ObjectRef]map[string]*schema.ForeignKey
}

// NewRelational returns an empty RelationalBuilder.
func NewRelational() *RelationalBuilder {
	return &RelationalBuilder{
		objs:    map[schema.ObjectRef]*schema.Object{},
		fkOrder: map[schema.ObjectRef][]string{},
		fks:     map[schema.ObjectRef]map[string]*schema.ForeignKey{},
	}
}

func (b *RelationalBuilder) object(ref schema.ObjectRef) *schema.Object {
	o, ok := b.objs[ref]
	if !ok {
		o = &schema.Object{Ref: ref, Relational: &schema.RelationalDetail{}}
		b.objs[ref] = o
		b.order = append(b.order, ref)
	}
	return o
}

// Ensure registers an object even if it has no columns yet.
func (b *RelationalBuilder) Ensure(ref schema.ObjectRef) { b.object(ref) }

// AddColumn appends a column to the object's relational facet.
func (b *RelationalBuilder) AddColumn(ref schema.ObjectRef, c schema.Column) {
	o := b.object(ref)
	o.Relational.Columns = append(o.Relational.Columns, c)
}

// AddPrimaryKeyColumn appends a column to the object's primary key (call order).
func (b *RelationalBuilder) AddPrimaryKeyColumn(ref schema.ObjectRef, col string) {
	o := b.object(ref)
	o.Relational.PrimaryKey = append(o.Relational.PrimaryKey, col)
}

// AddForeignKeyColumn appends a (column -> referenced column) pair to a named
// foreign key, creating it on first sight with the qualified target reference.
func (b *RelationalBuilder) AddForeignKeyColumn(ref schema.ObjectRef, fkName, col string, references schema.ObjectRef, refCol string) {
	b.object(ref)
	if b.fks[ref] == nil {
		b.fks[ref] = map[string]*schema.ForeignKey{}
	}
	fk, ok := b.fks[ref][fkName]
	if !ok {
		fk = &schema.ForeignKey{Name: fkName, References: references}
		b.fks[ref][fkName] = fk
		b.fkOrder[ref] = append(b.fkOrder[ref], fkName)
	}
	fk.Columns = append(fk.Columns, col)
	fk.ReferencedColumns = append(fk.ReferencedColumns, refCol)
}

// AddIndex appends an index to the object's relational facet.
func (b *RelationalBuilder) AddIndex(ref schema.ObjectRef, ix schema.SecondaryIndex) {
	o := b.object(ref)
	o.Relational.Indexes = append(o.Relational.Indexes, ix)
}

// Build attaches accumulated foreign keys and returns objects in first-seen order.
func (b *RelationalBuilder) Build() []schema.Object {
	out := make([]schema.Object, 0, len(b.order))
	for _, ref := range b.order {
		o := b.objs[ref]
		for _, fkName := range b.fkOrder[ref] {
			o.Relational.ForeignKeys = append(o.Relational.ForeignKeys, *b.fks[ref][fkName])
		}
		out = append(out, *o)
	}
	return out
}
