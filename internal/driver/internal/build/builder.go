// Package build assembles a *schema.Schema incrementally from the flat rows
// that each driver's introspection queries return. It is shared by the
// per-driver introspectors so they need not each reinvent row grouping.
package build

import "github.com/sqlwarden/internal/schema"

// Builder assembles a *schema.Schema incrementally from flat introspection rows.
// It is not safe for concurrent use; each Introspect call uses its own Builder.
type Builder struct {
	order   []string // namespace order
	nsIndex map[string]int
	schemas map[string]*nsAccum
}

type nsAccum struct {
	name    string
	tblOrd  []string
	tables  map[string]*schema.Table
	viewOrd []string
	views   map[string]*schema.View
	fkOrder map[string][]string // table -> fk name order
	fks     map[string]map[string]*schema.ForeignKey
}

// New returns an empty Builder.
func New() *Builder {
	return &Builder{nsIndex: map[string]int{}, schemas: map[string]*nsAccum{}}
}

func (b *Builder) ns(name string) *nsAccum {
	if a, ok := b.schemas[name]; ok {
		return a
	}
	a := &nsAccum{
		name:    name,
		tables:  map[string]*schema.Table{},
		views:   map[string]*schema.View{},
		fkOrder: map[string][]string{},
		fks:     map[string]map[string]*schema.ForeignKey{},
	}
	b.schemas[name] = a
	b.nsIndex[name] = len(b.order)
	b.order = append(b.order, name)
	return a
}

// AddColumn appends a column to a table (or view, when isView is true).
func (b *Builder) AddColumn(ns, table string, isView bool, c schema.Column) {
	a := b.ns(ns)
	if isView {
		v, ok := a.views[table]
		if !ok {
			v = &schema.View{Name: table}
			a.views[table] = v
			a.viewOrd = append(a.viewOrd, table)
		}
		v.Columns = append(v.Columns, c)
		return
	}
	t, ok := a.tables[table]
	if !ok {
		t = &schema.Table{Name: table}
		a.tables[table] = t
		a.tblOrd = append(a.tblOrd, table)
	}
	t.Columns = append(t.Columns, c)
}

// AddPrimaryKeyColumn appends a column to a table's primary key (in call order).
func (b *Builder) AddPrimaryKeyColumn(ns, table, col string) {
	t := b.ensureTable(ns, table)
	t.PrimaryKey = append(t.PrimaryKey, col)
}

// AddForeignKeyColumn appends a (column -> referenced column) pair to the named
// foreign key on a table, creating the foreign key on first sight.
func (b *Builder) AddForeignKeyColumn(ns, table, name, col, refTable, refCol string) {
	a := b.ns(ns)
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
	t := b.ensureTable(ns, table)
	t.Indexes = append(t.Indexes, ix)
}

func (b *Builder) ensureTable(ns, table string) *schema.Table {
	a := b.ns(ns)
	t, ok := a.tables[table]
	if !ok {
		t = &schema.Table{Name: table}
		a.tables[table] = t
		a.tblOrd = append(a.tblOrd, table)
	}
	return t
}

// Build finalizes the schema. database is the catalog/db name (may be empty).
func (b *Builder) Build(database string) *schema.Schema {
	out := &schema.Schema{Database: database}
	for _, nsName := range b.order {
		a := b.schemas[nsName]
		ns := schema.Namespace{Name: a.name}
		for _, tn := range a.tblOrd {
			t := a.tables[tn]
			for _, fkName := range a.fkOrder[tn] {
				t.ForeignKeys = append(t.ForeignKeys, *a.fks[tn][fkName])
			}
			ns.Tables = append(ns.Tables, *t)
		}
		for _, vn := range a.viewOrd {
			ns.Views = append(ns.Views, *a.views[vn])
		}
		out.Namespaces = append(out.Namespaces, ns)
	}
	return out
}
