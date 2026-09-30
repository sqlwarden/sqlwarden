package tidb

import (
	"context"

	"github.com/sqlwarden/internal/engine/engines/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.DefinitionInspector = (*driver)(nil)

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
// unreachable in practice since Tree never lists those kinds.
func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	if ref.Kind == "sequence" {
		return mysql.ShowCreateDefinition(ctx, d.DB(), ref, "SHOW CREATE SEQUENCE ", "Definition")
	}
	return d.Driver.InspectDefinition(ctx, ref)
}
