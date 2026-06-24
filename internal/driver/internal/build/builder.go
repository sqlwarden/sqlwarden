// Package build assembles the two-tier schema models from the flat rows that a
// driver's inspection queries return: a CatalogBuilder for the cheap listing,
// and a RelationalBuilder for typed object detail with qualified foreign keys.
// Neither is safe for concurrent use; each inspection uses its own builder.
package build

import "github.com/sqlwarden/internal/schema"

// CatalogBuilder accumulates object refs per namespace and emits them grouped by
// kind, in the order kinds were declared (undeclared kinds sort last, first-seen).
type CatalogBuilder struct {
	kindOrder []string
	kindSeen  map[string]bool
	nsOrder   []string
	ns        map[string]*nsCat
}

type nsCat struct {
	name       string
	groupSeen  map[string]bool
	groupOrder []string
	groups     map[string][]schema.ObjectRef
}

// NewCatalog returns an empty CatalogBuilder.
func NewCatalog() *CatalogBuilder {
	return &CatalogBuilder{kindSeen: map[string]bool{}, ns: map[string]*nsCat{}}
}

// DeclareKind fixes a kind's position in every namespace's group ordering.
func (b *CatalogBuilder) DeclareKind(kind string) {
	if !b.kindSeen[kind] {
		b.kindSeen[kind] = true
		b.kindOrder = append(b.kindOrder, kind)
	}
}

// AddRef records an object of the given kind in the given namespace.
func (b *CatalogBuilder) AddRef(namespace, kind, name string) {
	n, ok := b.ns[namespace]
	if !ok {
		n = &nsCat{name: namespace, groupSeen: map[string]bool{}, groups: map[string][]schema.ObjectRef{}}
		b.ns[namespace] = n
		b.nsOrder = append(b.nsOrder, namespace)
	}
	if !n.groupSeen[kind] {
		n.groupSeen[kind] = true
		n.groupOrder = append(n.groupOrder, kind)
	}
	n.groups[kind] = append(n.groups[kind], schema.ObjectRef{Namespace: namespace, Kind: kind, Name: name})
}

// Build finalizes the catalog with the given header fields.
func (b *CatalogBuilder) Build(connection, dialect, database string) *schema.Catalog {
	cat := &schema.Catalog{Connection: connection, Dialect: dialect, Database: database}
	for _, nsName := range b.nsOrder {
		n := b.ns[nsName]
		nc := schema.NamespaceCatalog{Name: n.name}
		emitted := map[string]bool{}
		emit := func(kind string) {
			refs := n.groups[kind]
			if len(refs) == 0 || emitted[kind] {
				return
			}
			emitted[kind] = true
			nc.Groups = append(nc.Groups, schema.ObjectGroupCatalog{Kind: kind, Objects: refs})
		}
		for _, kind := range b.kindOrder {
			emit(kind)
		}
		for _, kind := range n.groupOrder { // undeclared kinds, first-seen
			emit(kind)
		}
		cat.Namespaces = append(cat.Namespaces, nc)
	}
	return cat
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
func (b *RelationalBuilder) AddIndex(ref schema.ObjectRef, ix schema.Index) {
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
