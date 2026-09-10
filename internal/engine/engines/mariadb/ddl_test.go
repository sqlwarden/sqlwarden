package mariadb

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/engines/mysql"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestDefaultDecoderPassesRawValueThrough(t *testing.T) {
	tests := []struct {
		name string
		def  mysql.ColumnDefault
	}{
		// MariaDB reports a string default already quoted, unlike MySQL 8.
		{name: "quoted string", def: mysql.ColumnDefault{ColumnType: "varchar(50)", Raw: "'unnamed'"}},
		// MariaDB never writes DEFAULT_GENERATED into extra, so an expression
		// default is only recognisable by already being a valid expression.
		{name: "expression", def: mysql.ColumnDefault{ColumnType: "datetime", Extra: "", Raw: "current_timestamp()"}},
		{name: "bit literal", def: mysql.ColumnDefault{ColumnType: "bit(8)", Raw: "b'101'"}},
		{name: "embedded quote", def: mysql.ColumnDefault{ColumnType: "varchar(50)", Raw: "'it''s here'"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (defaultDecoder{}).DecodeColumnDefault(tt.def); got != tt.def.Raw {
				t.Errorf("got %q, want the raw value %q", got, tt.def.Raw)
			}
		})
	}
}

func mariaDBShowColumnDefinition(t *testing.T, d *driver, table, column string) string {
	t.Helper()
	var name, create string
	if err := d.DB().QueryRowContext(context.Background(), "SHOW CREATE TABLE `"+table+"`").Scan(&name, &create); err != nil {
		t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
	}
	for _, line := range strings.Split(create, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "`"+column+"` ") {
			return trimmed
		}
	}
	t.Fatalf("column %q not found in:\n%s", column, create)
	return ""
}

func mariaDBTestScope(t *testing.T, d *driver) metadata.ScopePath {
	t.Helper()
	var database string
	if err := d.DB().QueryRowContext(context.Background(), `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
}

// TestApplyDDLAlterColumnRoundTripsMariaDBDefaults proves ALTER COLUMN works
// against a live MariaDB server, which reports information_schema column
// defaults differently from MySQL 8: string defaults arrive already quoted
// (re-quoting them would corrupt the value) and expression defaults are not
// flagged in extra (treating them as literals makes MariaDB reject the
// statement with "Invalid default value"). The driver's own decoder is what
// makes both cases round trip; the inherited MySQL behaviour fails both.
func TestApplyDDLAlterColumnRoundTripsMariaDBDefaults(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	drop := func() {
		_, _ = d.DB().ExecContext(ctx, "DROP TABLE IF EXISTS ddl_maria_defaults")
	}
	drop()
	t.Cleanup(drop)

	if _, err := d.DB().ExecContext(ctx, `CREATE TABLE ddl_maria_defaults (
		id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
		label VARCHAR(50) COLLATE utf8mb4_bin NOT NULL DEFAULT 'unnamed' COMMENT 'the label',
		seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	ref := metadata.ObjectRef{Scope: mariaDBTestScope(t, d), Kind: "table", Name: "ddl_maria_defaults"}

	labelType := "varchar(100)"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "label",
		Changes: &ddl.ColumnChanges{DataType: &labelType},
	}); err != nil {
		t.Fatalf("alter label column: %v", err)
	}
	label := mariaDBShowColumnDefinition(t, d, "ddl_maria_defaults", "label")
	for _, want := range []string{"varchar(100)", "DEFAULT 'unnamed'", "COMMENT 'the label'", "utf8mb4_bin"} {
		if !strings.Contains(strings.ToLower(label), strings.ToLower(want)) {
			t.Errorf("expected %q in %s", want, label)
		}
	}

	nullable := true
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "seen",
		Changes: &ddl.ColumnChanges{Nullable: &nullable},
	}); err != nil {
		t.Fatalf("alter seen column: %v", err)
	}
	seen := mariaDBShowColumnDefinition(t, d, "ddl_maria_defaults", "seen")
	for _, want := range []string{"default current_timestamp", "on update current_timestamp"} {
		if !strings.Contains(strings.ToLower(seen), want) {
			t.Errorf("expected %q in %s", want, seen)
		}
	}

	idType := "bigint"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "id",
		Changes: &ddl.ColumnChanges{DataType: &idType},
	}); err != nil {
		t.Fatalf("alter id column: %v", err)
	}
	id := mariaDBShowColumnDefinition(t, d, "ddl_maria_defaults", "id")
	if !strings.Contains(strings.ToLower(id), "auto_increment") {
		t.Errorf("AUTO_INCREMENT was dropped: %s", id)
	}
}

// TestApplyDDLAlterColumnPreservesMariaDBGeneratedExpression proves a
// type-only edit does not drop a GENERATED ALWAYS AS (...) expression against
// a live MariaDB server, whose information_schema.columns.generation_expression
// is not self-parenthesized the way MySQL 8's is — this is the shared
// mysql.Driver code path MariaDB inherits unmodified, so it must round trip
// MariaDB's unparenthesized form too, not just MySQL's.
func TestApplyDDLAlterColumnPreservesMariaDBGeneratedExpression(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	drop := func() {
		_, _ = d.DB().ExecContext(ctx, "DROP TABLE IF EXISTS ddl_maria_generated")
	}
	drop()
	t.Cleanup(drop)

	if _, err := d.DB().ExecContext(ctx, `CREATE TABLE ddl_maria_generated (
		id INT PRIMARY KEY,
		price INT,
		tax INT GENERATED ALWAYS AS (price * 2) STORED
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	ref := metadata.ObjectRef{Scope: mariaDBTestScope(t, d), Kind: "table", Name: "ddl_maria_generated"}

	newType := "bigint"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "tax",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter generated column: %v", err)
	}
	definition := mariaDBShowColumnDefinition(t, d, "ddl_maria_generated", "tax")
	if !strings.Contains(strings.ToLower(definition), "generated always as") || !strings.Contains(strings.ToLower(definition), "stored") {
		t.Errorf("generated expression was dropped: %s", definition)
	}
	if !strings.Contains(strings.ToLower(definition), "bigint") {
		t.Errorf("expected the type to change to bigint: %s", definition)
	}
}
