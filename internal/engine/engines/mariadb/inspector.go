package mariadb

import (
	"context"

	"github.com/sqlwarden/internal/engine/engines/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.DefinitionInspector = (*driver)(nil)

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
// equivalent in the embedded mysql implementation), index reconstruction
// (MariaDB's information_schema.statistics lacks the EXPRESSION column the
// base implementation selects) and constraint reconstruction (MariaDB's
// information_schema.check_constraints carries table_name, needed because CHECK
// names are unique per table), and delegates every other kind to it unmodified.
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
	case "constraint":
		def, err := mysql.ConstraintDefinition(ctx, d.DB(), ref, true)
		if err != nil {
			return nil, err
		}
		return mysql.SourceDescriptor("DDL", def), nil
	}
	return d.Driver.InspectDefinition(ctx, ref)
}
