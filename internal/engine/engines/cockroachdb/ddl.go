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
	// CockroachDB's built-in types, minus Postgres constructs it doesn't
	// implement: the geometric types, tsvector/tsquery, xml, pg_lsn, and
	// range/multirange types.
	ColumnTypes: []string{
		"bigint", "bigserial", "bit", "bit varying", "boolean", "bytea",
		"char", "date", "decimal", "double precision", "inet",
		"int2", "int4", "int8", "integer", "interval", "json", "jsonb",
		"numeric", "oid", "real", "serial", "serial2", "serial4", "serial8",
		"smallint", "smallserial", "text", "time", "time with time zone",
		"timestamp", "timestamp with time zone", "timestamptz", "uuid", "varchar",
	},
	CreatableTableScopeKinds: []string{"schema"},
	DroppableObjectKinds:     []string{"table", "view"},
	DroppableScopeKinds:      []string{"schema"},
	SupportsCascade:          true,
	ParameterizedColumnTypes: []ddl.ParameterizedColumnType{
		{Name: "decimal", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 1000}, {Name: "scale", Min: 0, Max: 1000, Optional: true}}},
		{Name: "numeric", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 1, Max: 1000}, {Name: "scale", Min: 0, Max: 1000, Optional: true}}},
		{Name: "varchar", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "char", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "bit", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "bit varying", Parameters: []ddl.ColumnTypeParameter{{Name: "length", Min: 1, Max: 10485760}}},
		{Name: "time", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "time", Suffix: "with time zone", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "timestamp", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
		{Name: "timestamp", Suffix: "with time zone", Parameters: []ddl.ColumnTypeParameter{{Name: "precision", Min: 0, Max: 6}}},
	},
}

func (d *driver) DDLSpec() ddl.Spec {
	return cockroachdbDDLSpec
}
