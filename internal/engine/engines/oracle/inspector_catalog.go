package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/engine/metadata"
)

// Each query projects owner and object name before its display fields. All
// requested names are bound in one batch per kind; no catalog SQL comes from
// object names or client-supplied field names.
type oracleCatalogSpec struct {
	kind, title, from, owner, name string
	columns, labels                []string
}

var oracleCatalogSpecs = []oracleCatalogSpec{
	{
		kind: "synonym", title: "Synonym", from: "all_synonyms", owner: "owner", name: "synonym_name",
		columns: []string{"table_owner", "table_name", "db_link"},
		labels:  []string{"Target schema", "Target object", "Database link"},
	},
	{
		kind: "db_link", title: "Database link", from: "all_db_links", owner: "owner", name: "db_link",
		columns: []string{"username", "host"},
		labels:  []string{"Remote user", "Connect string"},
	},
	{
		kind: "index", title: "Index", from: "all_indexes", owner: "owner", name: "index_name",
		columns: []string{"table_owner", "table_name", "index_type", "uniqueness", "status", "tablespace_name", "partitioned"},
		labels:  []string{"Table schema", "Table", "Type", "Uniqueness", "Status", "Tablespace", "Partitioned"},
	},
	{
		kind: "constraint", title: "Constraint", from: "all_constraints", owner: "owner", name: "constraint_name",
		columns: []string{"table_name", "constraint_type", "status", "validated", "deferrable", "deferred", "r_owner", "r_constraint_name", "delete_rule", "search_condition"},
		labels:  []string{"Table", "Type", "Status", "Validation", "Deferrable", "Initially deferred", "Referenced schema", "Referenced constraint", "Delete rule", "Check condition"},
	},
}

func (d *oracleDriver) inspectCatalogObjects(ctx context.Context, refs []metadata.ObjectRef) ([]metadata.Object, error) {
	objects := make(map[metadata.ObjectRef]metadata.Object, len(refs))
	for _, spec := range oracleCatalogSpecs {
		var batch []metadata.ObjectRef
		for _, ref := range refs {
			if ref.Kind == spec.kind {
				batch = append(batch, ref)
			}
		}
		if len(batch) == 0 {
			continue
		}
		filter, args := oracleDict{}.objFilter(spec.owner, spec.name, batch, 1)
		query := "SELECT " + spec.owner + ", " + spec.name + ", " + strings.Join(spec.columns, ", ") +
			" FROM " + spec.from + " WHERE " + filter + " ORDER BY " + spec.owner + ", " + spec.name
		rows, err := d.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("oracle: %s detail: %w", spec.kind, err)
		}
		for rows.Next() {
			var owner, name string
			values := make([]sql.NullString, len(spec.columns))
			dest := []any{&owner, &name}
			for i := range values {
				dest = append(dest, &values[i])
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("oracle: %s detail scan: %w", spec.kind, err)
			}
			fields := make([]metadata.Field, 0, len(values))
			for i, value := range values {
				if value.Valid && value.String != "" {
					fields = append(fields, metadata.Field{Name: spec.labels[i], Value: value.String})
				}
			}
			ref := metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: owner}), Kind: spec.kind, Name: name}
			objects[ref] = metadata.Object{Ref: ref, Descriptors: []metadata.Descriptor{{Kind: "fields", Title: spec.title, Fields: fields}}}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("oracle: %s detail rows: %w", spec.kind, err)
		}
	}
	result := make([]metadata.Object, 0, len(objects))
	for _, ref := range refs {
		if object, ok := objects[ref]; ok {
			result = append(result, object)
		}
	}
	return result, nil
}
