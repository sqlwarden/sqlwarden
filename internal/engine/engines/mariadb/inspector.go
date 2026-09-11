package mariadb

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

// SchemaSpec extends the embedded mysql spec with MariaDB's native sequence
// object kind.
func (d *driver) SchemaSpec() metadata.SchemaSpec {
	spec := d.Driver.SchemaSpec()
	spec.Dialect = "mariadb"
	spec.Kinds = append(spec.Kinds, metadata.SchemaObjectKind{
		Kind: "sequence", Label: "Sequence", PluralLabel: "Sequences",
		Order: len(spec.Kinds) + 1, Relational: false, SupportsDiagram: false, Listing: "enumerated",
		HasDefinition: true,
	})
	return spec
}

// InspectDirectory composes mariadb.CatalogTables/CatalogSequences with the
// embedded driver's AttachRowCounts/CatalogRoutines/CatalogTriggers. It
// cannot simply delegate to mysql.Driver.InspectDirectory and add sequences
// afterward: the base implementation's table query has no way to know about
// MariaDB's SEQUENCE table_type, so it would report every sequence as a
// plain table too.
func (d *driver) InspectDirectory(ctx context.Context, opts metadata.DirectoryOptions) (*metadata.Directory, error) {
	database := opts.Root.Name("database")
	if database == "" {
		database = d.DefaultScope().Name("database")
	}
	if database == "" {
		var current sql.NullString
		if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&current); err != nil {
			return nil, fmt.Errorf("mariadb: directory database name: %w", err)
		}
		database = current.String
	}
	if database == "" {
		return &metadata.Directory{Engine: "mariadb"}, nil
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	b := build.NewDirectory()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.DeclareKind("function")
	b.DeclareKind("procedure")
	b.DeclareKind("trigger")
	b.DeclareKind("sequence")
	b.DeclareKind("index")
	b.DeclareKind("constraint")

	if err := CatalogTables(ctx, d.DB(), database, func(ns, name, kind string) { b.AddRef(scope, kind, name) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog tables: %w", err)
	}
	if err := mysql.AttachRowCounts(ctx, d.DB(), database, func(name string, count int64) { b.SetRowCount(scope, "table", name, count) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog row counts: %w", err)
	}
	if err := mysql.CatalogRoutines(ctx, d.DB(), database, func(ns, name, kind string) { b.AddRef(scope, kind, name) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog routines: %w", err)
	}
	if err := mysql.CatalogTriggers(ctx, d.DB(), database, func(ns, name string) { b.AddRef(scope, "trigger", name) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog triggers: %w", err)
	}
	if err := CatalogSequences(ctx, d.DB(), database, func(ns, name string) { b.AddRef(scope, "sequence", name) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog sequences: %w", err)
	}
	if err := mysql.CatalogIndexes(ctx, d.DB(), database, func(ns, name string) { b.AddRef(scope, "index", name) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog indexes: %w", err)
	}
	if err := mysql.CatalogConstraints(ctx, d.DB(), database, func(ns, name string) { b.AddRef(scope, "constraint", name) }); err != nil {
		return nil, fmt.Errorf("mariadb: catalog constraints: %w", err)
	}

	return b.Build("", "mariadb", scope), nil
}

// InspectObjects delegates table/view/function/procedure/trigger refs to the
// embedded mysql implementation, patching relational columns for MariaDB's
// JSON-as-LONGTEXT reporting, and serves sequence refs from mariadb.SequenceObjects.
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
		if err := patchJSONColumns(ctx, d.DB(), otherRefs, objs); err != nil {
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
// equivalent in the embedded mysql implementation) and index reconstruction
// (MariaDB's information_schema.statistics lacks the EXPRESSION column the
// base implementation selects), and delegates every other kind to it unmodified.
func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	switch ref.Kind {
	case "sequence":
		return mysql.ShowCreateDefinition(ctx, d.DB(), ref, "SHOW CREATE SEQUENCE ", "Definition")
	case "index":
		def, err := mysql.IndexDefinition(ctx, d.DB(), ref, false)
		if err != nil {
			return nil, err
		}
		return mysql.SourceDescriptor("DDL", def), nil
	}
	return d.Driver.InspectDefinition(ctx, ref)
}
