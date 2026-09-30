package cockroachdb

import (
	"context"
	"database/sql"

	"github.com/sqlwarden/internal/engine/engines/postgres"
	"github.com/sqlwarden/internal/engine/metadata"
)

// InspectObjects uses postgres detail for every kind except function and
// procedure, whose signature and language CockroachDB only exposes through
// pg_get_functiondef.
func (d *driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	return d.InspectObjectsWith(ctx, refs, inspectObjectsIn)
}

func inspectObjectsIn(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	routines := map[string][]metadata.ObjectRef{}
	var rest []metadata.ObjectRef
	for _, r := range refs {
		if _, ok := routineKinds[r.Kind]; ok {
			routines[r.Kind] = append(routines[r.Kind], r)
		} else {
			rest = append(rest, r)
		}
	}
	out, err := postgres.InspectObjectsIn(ctx, db, rest)
	if err != nil {
		return nil, err
	}
	for kind, kindRefs := range routines {
		objs, err := functionObjects(ctx, db, kind, kindRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition delegates every kind except function and procedure to
// postgres.Driver. Those are overridden because functionDefinition (catalog.go) must
// recover the language from pg_get_functiondef's own LANGUAGE clause rather
// than postgres.FunctionDefinition's pg_language join, which always returns
// zero rows on CockroachDB.
func (d *driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	if _, ok := routineKinds[ref.Kind]; !ok {
		return d.Driver.InspectDefinition(ctx, ref)
	}
	q, err := d.Querier(ctx, ref.Scope.Name("database"))
	if err != nil {
		return nil, err
	}
	language, def, err := functionDefinition(ctx, q, ref)
	if err != nil {
		return nil, err
	}
	return postgres.SourceDescriptor("Definition", language, def), nil
}
