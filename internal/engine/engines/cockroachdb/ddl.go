package cockroachdb

import (
	"github.com/sqlwarden/internal/engine/ddl"
)

var _ ddl.Executor = (*driver)(nil)

// cockroachdbDDLSpec matches the embedded postgres.Driver's spec minus
// materialized_view: CockroachDB has no CREATE MATERIALIZED VIEW support, so
// there is nothing of that kind to drop. ApplyDDL is inherited unmodified
// from postgres.Driver — every operation it implements (CREATE TABLE, DROP
// TABLE/VIEW, DROP SCHEMA, column rename/drop, DROP INDEX) uses syntax
// CockroachDB also accepts.
var cockroachdbDDLSpec = ddl.Spec{
	Operations: []ddl.Operation{
		ddl.OperationCreateTable,
		ddl.OperationDropObject,
		ddl.OperationDropScope,
		ddl.OperationRenameColumn,
		ddl.OperationDropColumn,
		ddl.OperationDropIndex,
	},
	ColumnTypes: []string{
		"bigint", "bigserial", "boolean", "bytea", "date", "double precision",
		"integer", "json", "jsonb", "numeric", "real", "smallint", "serial",
		"text", "time", "timestamp", "timestamp with time zone", "uuid", "varchar",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view"},
	DroppableScopeKinds:      []string{"schema"},
	SupportsCascade:          true,
}

func (d *driver) DDLSpec() ddl.Spec {
	return cockroachdbDDLSpec
}
