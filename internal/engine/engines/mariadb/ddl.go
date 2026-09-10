package mariadb

import (
	"context"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/engines/mysql"
)

var _ ddl.Executor = (*driver)(nil)

// defaultDecoder reads MariaDB's information_schema.columns.column_default,
// which — unlike MySQL 8's — is already a complete SQL expression for every
// kind of default: string literals arrive quoted ('unnamed'), bit literals as
// b'1', and expression defaults as the function call MariaDB normalised them
// to (current_timestamp()). MariaDB also never writes DEFAULT_GENERATED into
// extra, so MySQL's marker-based literal/expression discrimination cannot work
// here. Re-quoting any of these would corrupt the value or make MariaDB reject
// the redeclaration with "Invalid default value", so the raw value is passed
// through untouched.
type defaultDecoder struct{}

func (defaultDecoder) DecodeColumnDefault(def mysql.ColumnDefault) string {
	return def.Raw
}

// SuppressGeneratedNullability reports true: MariaDB rejects MODIFY COLUMN
// on a GENERATED column carrying an explicit NULL or NOT NULL clause,
// whether the column is nullable or not, unlike MySQL 8 which accepts (and
// requires, to preserve NOT NULL) the clause.
func (defaultDecoder) SuppressGeneratedNullability() bool { return true }

// ApplyDDL delegates to the embedded MySQL implementation with MariaDB's own
// column-default decoding. The override is required because Go method
// promotion has no virtual dispatch: without it, ALTER COLUMN would run the
// embedded implementation's MySQL 8 assumptions against a MariaDB server.
func (d *driver) ApplyDDL(ctx context.Context, request ddl.Request) error {
	return d.Driver.ApplyDDLWithDefaults(ctx, request, defaultDecoder{})
}
