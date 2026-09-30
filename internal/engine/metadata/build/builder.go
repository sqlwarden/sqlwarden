// Package build assembles typed relational object detail with qualified foreign
// keys from the flat rows that a driver's inspection queries return. A
// RelationalBuilder is not safe for concurrent use; each inspection uses its own.
package build

import "github.com/sqlwarden/internal/engine/metadata"

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
