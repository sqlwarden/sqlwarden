package mysql

import (
	"context"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.DefinitionInspector = (*Driver)(nil)

// InspectObjects buckets refs by kind and composes RelationalObjects,
// RoutineObjects, TriggerObjects, and EventObjects from catalog.go. A compatible engine
// overrides this method entirely to drop or add kinds.
func (d *Driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs []metadata.ObjectRef
	var routineRefs []metadata.ObjectRef
	var triggerRefs []metadata.ObjectRef
	var eventRefs []metadata.ObjectRef
	var idxRefs []metadata.ObjectRef
	var conRefs []metadata.ObjectRef
	for _, ref := range refs {
		switch ref.Kind {
		case "table", "view":
			relRefs = append(relRefs, ref)
		case "function", "procedure":
			routineRefs = append(routineRefs, ref)
		case "trigger":
			triggerRefs = append(triggerRefs, ref)
		case "event":
			eventRefs = append(eventRefs, ref)
		case "index":
			idxRefs = append(idxRefs, ref)
		case "constraint":
			conRefs = append(conRefs, ref)
		}
	}

	var out []metadata.Object
	if len(relRefs) > 0 {
		objs, err := RelationalObjects(ctx, d.db, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(routineRefs) > 0 {
		objs, err := RoutineObjects(ctx, d.db, routineRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(triggerRefs) > 0 {
		objs, err := TriggerObjects(ctx, d.db, triggerRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(eventRefs) > 0 {
		objs, err := EventObjects(ctx, d.db, eventRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(idxRefs) > 0 {
		objs, err := IndexObjects(ctx, d.db, idxRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(conRefs) > 0 {
		objs, err := ConstraintObjects(ctx, d.db, conRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition serves one object's canonical text definition on demand via
// SHOW CREATE, so bulk InspectObjects (and every schema snapshot) skips the
// per-object SHOW CREATE TABLE round trip and the routine-body column it used to
// carry. Tables yield a "DDL" descriptor; views and routines yield "Definition".
// Index and constraint DDL are reconstructed from information_schema, since
// MySQL has no SHOW CREATE for them. Unsupported kinds (e.g. triggers), or an
// object that no longer exists, yield a nil descriptor with a nil error. A
// compatible engine (e.g. MariaDB adding a "sequence" case) overrides this
// method, handles its own kinds, and delegates everything else to this default
// via d.Driver.InspectDefinition(ctx, ref).
func (d *Driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	switch ref.Kind {
	case "index":
		def, err := IndexDefinition(ctx, d.db, ref, true)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", def), nil
	case "constraint":
		def, err := ConstraintDefinition(ctx, d.db, ref, false)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", def), nil
	}
	var stmt, title string
	switch ref.Kind {
	case "table":
		stmt, title = "SHOW CREATE TABLE ", "DDL"
	case "view":
		stmt, title = "SHOW CREATE VIEW ", "Definition"
	case "function":
		stmt, title = "SHOW CREATE FUNCTION ", "Definition"
	case "procedure":
		stmt, title = "SHOW CREATE PROCEDURE ", "Definition"
	case "event":
		stmt, title = "SHOW CREATE EVENT ", "Definition"
	default:
		return nil, nil
	}
	return ShowCreateDefinition(ctx, d.db, ref, stmt, title)
}
