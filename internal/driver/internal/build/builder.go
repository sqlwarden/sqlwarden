// Package build assembles a *schema.Schema incrementally from the flat rows
// that each driver's introspection queries return. It is shared by the
// per-driver introspectors so they need not each reinvent row grouping.
//
// Objects are accumulated by kind and emitted as ordered ObjectGroups. The
// contract for supporting a new object type (this database or a future driver)
// is small:
//
//	b.DeclareGroup("function", "Functions") // heading + position
//	b.AddObject(ns, "function", name)        // a column-less object
//	b.AddObjectColumn(ns, "view", name, col) // an object with columns
//	b.SetObjectAttribute(ns, "sequence", name, "increment", 1) // kind-specific
//
// table/view are pre-declared so the existing relational drivers need no change.
package build

import "github.com/sqlwarden/internal/schema"

// Universal relational object kinds, pre-declared by New().
const (
	KindTable = "table"
	KindView  = "view"
)

// Builder assembles a *schema.Schema incrementally from flat introspection rows.
// It is not safe for concurrent use; each Introspect call uses its own Builder.
type Builder struct {
	order      []string // namespace order
	schemas    map[string]*nsAccum
	groupOrder []string          // object-group kind order (driver-declared)
	labels     map[string]string // kind -> heading
}

type nsAccum struct {
	name    string
	objOrd  map[string][]string                  // kind -> object name order
	objs    map[string]map[string]*schema.Object // kind -> name -> object
	fkOrder map[string][]string                  // table -> fk name order
	fks     map[string]map[string]*schema.ForeignKey
}

// New returns a Builder with the universal relational kinds (table, view)
// pre-declared. Drivers DeclareGroup any additional kinds in the order they
// should appear.
func New() *Builder {
	b := &Builder{schemas: map[string]*nsAccum{}, labels: map[string]string{}}
	b.DeclareGroup(KindTable, "Tables")
	b.DeclareGroup(KindView, "Views")
	return b
}

// DeclareGroup registers an object-group kind with its heading and append
// position. Re-declaring updates the label without changing position.
func (b *Builder) DeclareGroup(kind, label string) {
	if _, ok := b.labels[kind]; !ok {
		b.groupOrder = append(b.groupOrder, kind)
	}
	b.labels[kind] = label
}

func (b *Builder) ns(name string) *nsAccum {
	if a, ok := b.schemas[name]; ok {
		return a
	}
	a := &nsAccum{
		name:    name,
		objOrd:  map[string][]string{},
		objs:    map[string]map[string]*schema.Object{},
		fkOrder: map[string][]string{},
		fks:     map[string]map[string]*schema.ForeignKey{},
	}
	b.schemas[name] = a
	b.order = append(b.order, name)
	return a
}

func (b *Builder) obj(ns, kind, name string) *schema.Object {
	a := b.ns(ns)
	if a.objs[kind] == nil {
		a.objs[kind] = map[string]*schema.Object{}
	}
	o, ok := a.objs[kind][name]
	if !ok {
		o = &schema.Object{Name: name}
		a.objs[kind][name] = o
		a.objOrd[kind] = append(a.objOrd[kind], name)
	}
	return o
}

// AddObject registers a column-less object of the given kind (e.g. a function,
// sequence, or trigger).
func (b *Builder) AddObject(ns, kind, name string) { b.obj(ns, kind, name) }

// AddObjectColumn appends a column to an object of the given kind.
func (b *Builder) AddObjectColumn(ns, kind, name string, c schema.Column) {
	o := b.obj(ns, kind, name)
	o.Columns = append(o.Columns, c)
}

// SetObjectAttribute records a kind-specific detail (function signature,
// sequence increment, …) on an object.
func (b *Builder) SetObjectAttribute(ns, kind, name, key string, value any) {
	o := b.obj(ns, kind, name)
	if o.Attributes == nil {
		o.Attributes = map[string]any{}
	}
	o.Attributes[key] = value
}

// AddColumn appends a column to a table (or view when isView is true). Thin
// convenience over AddObjectColumn for the common relational case.
func (b *Builder) AddColumn(ns, name string, isView bool, c schema.Column) {
	kind := KindTable
	if isView {
		kind = KindView
	}
	b.AddObjectColumn(ns, kind, name, c)
}

// AddPrimaryKeyColumn appends a column to a table's primary key (in call order).
func (b *Builder) AddPrimaryKeyColumn(ns, table, col string) {
	o := b.obj(ns, KindTable, table)
	o.PrimaryKey = append(o.PrimaryKey, col)
}

// AddForeignKeyColumn appends a (column -> referenced column) pair to a table's
// named foreign key, creating the foreign key on first sight.
func (b *Builder) AddForeignKeyColumn(ns, table, name, col, refTable, refCol string) {
	a := b.ns(ns)
	b.obj(ns, KindTable, table) // ensure the table exists even with no columns yet
	if a.fks[table] == nil {
		a.fks[table] = map[string]*schema.ForeignKey{}
	}
	fk, ok := a.fks[table][name]
	if !ok {
		fk = &schema.ForeignKey{Name: name, ReferencedTable: refTable}
		a.fks[table][name] = fk
		a.fkOrder[table] = append(a.fkOrder[table], name)
	}
	fk.Columns = append(fk.Columns, col)
	fk.ReferencedColumns = append(fk.ReferencedColumns, refCol)
}

// AddIndex appends an index to a table.
func (b *Builder) AddIndex(ns, table string, ix schema.Index) {
	o := b.obj(ns, KindTable, table)
	o.Indexes = append(o.Indexes, ix)
}

// Build finalizes the schema. database is the catalog/db name (may be empty).
func (b *Builder) Build(database string) *schema.Schema {
	out := &schema.Schema{Database: database}
	for _, nsName := range b.order {
		a := b.schemas[nsName]
		ns := schema.Namespace{Name: a.name}

		// Attach accumulated foreign keys to their tables.
		for tbl, fkNames := range a.fkOrder {
			t := a.objs[KindTable][tbl]
			if t == nil {
				continue
			}
			for _, fkName := range fkNames {
				t.ForeignKeys = append(t.ForeignKeys, *a.fks[tbl][fkName])
			}
		}

		seen := map[string]bool{}
		emit := func(kind, label string) {
			names := a.objOrd[kind]
			if len(names) == 0 {
				return
			}
			seen[kind] = true
			g := schema.ObjectGroup{Kind: kind, Label: label}
			for _, n := range names {
				g.Objects = append(g.Objects, *a.objs[kind][n])
			}
			ns.ObjectGroups = append(ns.ObjectGroups, g)
		}

		for _, kind := range b.groupOrder {
			emit(kind, b.labels[kind])
		}
		// Any kinds emitted without a DeclareGroup (forward-compatibility) go
		// last, using the kind itself as the heading.
		for kind := range a.objs {
			if !seen[kind] {
				emit(kind, kind)
			}
		}

		out.Namespaces = append(out.Namespaces, ns)
	}
	return out
}
