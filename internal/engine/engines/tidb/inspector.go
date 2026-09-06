package tidb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/engines/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
	build "github.com/sqlwarden/internal/engine/metadata/build"
)

var _ metadata.SchemaInspector = (*driver)(nil)
var _ metadata.DefinitionInspector = (*driver)(nil)

// SchemaSpec drops function, procedure, and trigger from the embedded mysql
// spec — TiDB has no stored routine or trigger support — and adds TiDB's
// native sequence object kind.
func (d *driver) SchemaSpec() metadata.SchemaSpec {
	return metadata.SchemaSpec{
		Dialect: "tidb",
		Kinds: []metadata.SchemaObjectKind{
			{Kind: "table", Label: "Table", PluralLabel: "Tables", Order: 1, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "view", Label: "View", PluralLabel: "Views", Order: 2, Relational: true, SupportsDiagram: true, Listing: "enumerated"},
			{Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences", Order: 3, Relational: false, SupportsDiagram: false, Listing: "enumerated"},
		},
	}
}

// InspectDirectory composes tidb.CatalogTables/CatalogSequences with the
// embedded driver's AttachRowCounts. It cannot simply delegate to
// mysql.Driver.InspectDirectory and add sequences afterward: the base
// implementation's table query has no way to know about TiDB's SEQUENCE
// table_type, so it would report every sequence as a plain table too. It
// also omits CatalogRoutines/CatalogTriggers entirely, since TiDB has no
// functions, procedures, or triggers to enumerate.
func (d *driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	database := opts.Root.Name("database")
	if database == "" {
		database = d.DefaultScope().Name("database")
	}
	if database == "" {
		var current sql.NullString
		if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&current); err != nil {
			return nil, fmt.Errorf("tidb: directory database name: %w", err)
		}
		database = current.String
	}
	if database == "" {
		return &metadata.Directory{Engine: "tidb"}, nil
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	b := build.NewDirectory()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("sequence")

	if err := CatalogTables(ctx, d.DB(), database, func(ns, name, kind string) { b.AddRef(scope, kind, name) }); err != nil {
		return nil, fmt.Errorf("tidb: catalog tables: %w", err)
	}
	if err := mysql.AttachRowCounts(ctx, d.DB(), database, func(name string, count int64) { b.SetRowCount(scope, "table", name, count) }); err != nil {
		return nil, fmt.Errorf("tidb: catalog row counts: %w", err)
	}
	if err := CatalogSequences(ctx, d.DB(), database, func(ns, name string) { b.AddRef(scope, "sequence", name) }); err != nil {
		return nil, fmt.Errorf("tidb: catalog sequences: %w", err)
	}

	return b.Build("", "tidb", scope), nil
}

// InspectObjects delegates table/view refs to the embedded mysql
// implementation and serves sequence refs from tidb.SequenceObjects.
func (d *driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var seqRefs, otherRefs []metadata.ObjectRef
	for _, ref := range refs {
		if ref.Kind == "sequence" {
			seqRefs = append(seqRefs, ref)
		} else {
			otherRefs = append(otherRefs, ref)
		}
	}

	var out []metadata.Object
	if len(otherRefs) > 0 {
		objs, err := d.Driver.InspectObjects(ctx, otherRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(seqRefs) > 0 {
		objs, err := SequenceObjects(ctx, d.DB(), seqRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition special-cases sequences (SHOW CREATE SEQUENCE has no
// equivalent in the embedded mysql implementation) and delegates every other
// kind to it unmodified. The embedded default's function/procedure cases are
// unreachable in practice since InspectDirectory never emits those kinds.
func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	if ref.Kind == "sequence" {
		return mysql.ShowCreateDefinition(ctx, d.DB(), ref, "SHOW CREATE SEQUENCE ", "Definition")
	}
	return d.Driver.InspectDefinition(ctx, ref)
}
