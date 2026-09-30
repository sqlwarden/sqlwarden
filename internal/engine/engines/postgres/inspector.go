package postgres

import (
	"context"
	"database/sql"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.DefinitionInspector = (*Driver)(nil)

// InspectObjects buckets refs by kind and composes RelationalObjects,
// MaterializedViewObjects, FunctionObjects, SequenceObjects, ProcedureObjects,
// TriggerObjects, TypeObjects, DomainObjects, and ForeignTableObjects from
// catalog.go and inspector_objects.go. A compatible engine overrides this
// method with InspectObjectsWith and its own per-database inspect function.
func (d *Driver) InspectObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	return d.InspectObjectsWith(ctx, refs, InspectObjectsIn)
}

// InspectObjectsWith runs inspect once per database the refs span, against
// that database's pool, and qualifies results with the database scope.
// Compatible engines pass their own inspect to add or replace kinds.
func (d *Driver) InspectObjectsWith(ctx context.Context, refs []metadata.ObjectRef,
	inspect func(context.Context, *sql.DB, []metadata.ObjectRef) ([]metadata.Object, error)) ([]metadata.Object, error) {
	var out []metadata.Object
	for database, group := range groupRefsByDatabase(refs) {
		db, err := d.databaseFor(ctx, database)
		if err != nil {
			return nil, err
		}
		objs, err := inspect(ctx, db, group)
		if err != nil {
			return nil, err
		}
		qualifyObjects(objs, database)
		out = append(out, objs...)
	}
	return out, nil
}

func InspectObjectsIn(ctx context.Context, db *sql.DB, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	var relRefs, mvRefs, fnRefs, seqRefs, procRefs, trigRefs, typeRefs, domainRefs, foreignRefs []metadata.ObjectRef
	for _, r := range refs {
		switch r.Kind {
		case "table", "view":
			relRefs = append(relRefs, r)
		case "materialized_view":
			mvRefs = append(mvRefs, r)
		case "function":
			fnRefs = append(fnRefs, r)
		case "sequence":
			seqRefs = append(seqRefs, r)
		case "procedure":
			procRefs = append(procRefs, r)
		case "trigger":
			trigRefs = append(trigRefs, r)
		case "type":
			typeRefs = append(typeRefs, r)
		case "domain":
			domainRefs = append(domainRefs, r)
		case "foreign_table":
			foreignRefs = append(foreignRefs, r)
		}
	}

	var out []metadata.Object
	if len(relRefs) > 0 {
		objs, err := RelationalObjects(ctx, db, relRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(mvRefs) > 0 {
		objs, err := MaterializedViewObjects(ctx, db, mvRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(fnRefs) > 0 {
		objs, err := FunctionObjects(ctx, db, fnRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(seqRefs) > 0 {
		objs, err := SequenceObjects(ctx, db, seqRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(procRefs) > 0 {
		objs, err := ProcedureObjects(ctx, db, procRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(trigRefs) > 0 {
		objs, err := TriggerObjects(ctx, db, trigRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(typeRefs) > 0 {
		objs, err := TypeObjects(ctx, db, typeRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(domainRefs) > 0 {
		objs, err := DomainObjects(ctx, db, domainRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	if len(foreignRefs) > 0 {
		objs, err := ForeignTableObjects(ctx, db, foreignRefs)
		if err != nil {
			return nil, err
		}
		out = append(out, objs...)
	}
	return out, nil
}

// InspectDefinition serves one object's canonical text definition on demand so
// bulk InspectObjects (and every schema snapshot) skips the per-object cost:
// table, sequence, domain, type, and foreign table DDL are reconstructed from
// the catalog; views and materialized views come from pg_get_viewdef, functions
// and procedures from pg_get_functiondef, and triggers from pg_get_triggerdef.
// An unrecognized kind, or an object that no longer exists, yields a nil
// descriptor with a nil error. A compatible engine
// overrides this method to special-case a kind and delegate everything else to
// this default via d.Driver.InspectDefinition(ctx, ref).
func (d *Driver) InspectDefinition(ctx context.Context, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	db, err := d.databaseFor(ctx, ref.Scope.Name("database"))
	if err != nil {
		return nil, err
	}
	return inspectDefinitionIn(ctx, db, ref)
}

func inspectDefinitionIn(ctx context.Context, db *sql.DB, ref metadata.ObjectRef) (*metadata.Descriptor, error) {
	switch ref.Kind {
	case "table":
		ddl, err := TableDDL(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", ddl), nil
	case "view":
		def, err := ViewDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("Definition", "sql", def), nil
	case "materialized_view":
		def, err := MaterializedViewDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", def), nil
	case "function":
		language, def, err := FunctionDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("Definition", language, def), nil
	case "procedure":
		language, def, err := ProcedureDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("Definition", language, def), nil
	case "trigger":
		def, err := TriggerDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("Definition", "sql", def), nil
	case "sequence":
		def, err := SequenceDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", def), nil
	case "domain":
		def, err := DomainDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", def), nil
	case "type":
		def, err := TypeDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", def), nil
	case "foreign_table":
		def, err := ForeignTableDefinition(ctx, db, ref)
		if err != nil {
			return nil, err
		}
		return SourceDescriptor("DDL", "sql", def), nil
	default:
		return nil, nil
	}
}
