package oracle

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

func (d *oracleDriver) attachOracleTableDetails(ctx context.Context, objects []metadata.Object) error {
	var refs []metadata.ObjectRef
	byName := map[string]*metadata.Object{}
	for i := range objects {
		object := &objects[i]
		if object.Ref.Kind != "table" {
			continue
		}
		refs = append(refs, object.Ref)
		byName[object.Ref.Scope.Name("schema")+"\x00"+object.Ref.Name] = object
	}
	if len(refs) == 0 {
		return nil
	}
	filter, args := oracleDict{}.objFilter("owner", "table_name", refs, 1)
	rows, err := d.db.QueryContext(ctx, `SELECT owner, table_name, tablespace_name, partitioned, temporary, logging, compression
FROM all_tables WHERE `+filter, args...)
	if err != nil {
		return fmt.Errorf("oracle: table storage: %w", err)
	}
	for rows.Next() {
		var owner, name string
		var tablespace, partitioned, temporary, logging, compression sql.NullString
		if err := rows.Scan(&owner, &name, &tablespace, &partitioned, &temporary, &logging, &compression); err != nil {
			rows.Close()
			return fmt.Errorf("oracle: table storage scan: %w", err)
		}
		if object := byName[owner+"\x00"+name]; object != nil {
			setObjectAttr(object, "tablespace", tablespace.String)
			setObjectAttr(object, "partitioned", partitioned.String)
			object.Descriptors = append(object.Descriptors, metadata.Descriptor{
				Kind: "fields", Title: "Storage", Fields: []metadata.Field{
					{Name: "Tablespace", Value: tablespace.String},
					{Name: "Partitioned", Value: partitioned.String},
					{Name: "Temporary", Value: temporary.String},
					{Name: "Logging", Value: logging.String},
					{Name: "Compression", Value: compression.String},
				},
			})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("oracle: table storage rows: %w", err)
	}

	rows, err = d.db.QueryContext(ctx, `SELECT owner, table_name, column_name, generation_type
FROM all_tab_identity_cols WHERE `+filter, args...)
	if err != nil {
		return fmt.Errorf("oracle: identity columns: %w", err)
	}
	for rows.Next() {
		var owner, name, column, generation string
		if err := rows.Scan(&owner, &name, &column, &generation); err != nil {
			rows.Close()
			return fmt.Errorf("oracle: identity columns scan: %w", err)
		}
		if object := byName[owner+"\x00"+name]; object != nil && object.Relational != nil {
			for i := range object.Relational.Columns {
				if object.Relational.Columns[i].Name == column {
					setColumnAttr(&object.Relational.Columns[i], "identity", generation)
				}
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("oracle: identity columns rows: %w", err)
	}

	partitionFilter, partitionArgs := oracleDict{}.objFilter("table_owner", "table_name", refs, 1)
	rows, err = d.db.QueryContext(ctx, `SELECT table_owner, table_name, partition_name, tablespace_name, num_rows, high_value
FROM all_tab_partitions WHERE `+partitionFilter+` ORDER BY table_owner, table_name, partition_position`, partitionArgs...)
	if err != nil {
		return fmt.Errorf("oracle: table partitions: %w", err)
	}
	defer rows.Close()
	partitions := map[string]*metadata.RowSet{}
	for rows.Next() {
		var owner, name, partition string
		var tablespace, count, bound sql.NullString
		if err := rows.Scan(&owner, &name, &partition, &tablespace, &count, &bound); err != nil {
			return fmt.Errorf("oracle: table partitions scan: %w", err)
		}
		key := owner + "\x00" + name
		set := partitions[key]
		if set == nil {
			set = &metadata.RowSet{Columns: []string{"Name", "Tablespace", "Estimated rows", "High value"}}
			partitions[key] = set
		}
		set.Rows = append(set.Rows, []string{partition, tablespace.String, count.String, bound.String})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("oracle: table partitions rows: %w", err)
	}
	for key, set := range partitions {
		if object := byName[key]; object != nil {
			object.Descriptors = append(object.Descriptors, metadata.Descriptor{Kind: "rows", Title: "Partitions", Rows: set})
		}
	}
	return nil
}
