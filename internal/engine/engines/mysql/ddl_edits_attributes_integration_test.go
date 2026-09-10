package mysql

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

// mysqlShowColumnDefinition returns the SHOW CREATE TABLE line declaring
// column, which is the authoritative rendering of every attribute the column
// carries — including the ones (AUTO_INCREMENT, COMMENT, COLLATE, ON UPDATE)
// that InspectObjects does not surface.
func mysqlShowColumnDefinition(t *testing.T, d *Driver, table, column string) string {
	t.Helper()
	var name, create string
	if err := d.DB().QueryRowContext(context.Background(), "SHOW CREATE TABLE "+mysqlQuoteIdent(table)).Scan(&name, &create); err != nil {
		t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
	}
	for _, line := range strings.Split(create, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, mysqlQuoteIdent(column)+" ") {
			return trimmed
		}
	}
	t.Fatalf("column %q not found in:\n%s", column, create)
	return ""
}

// TestMySQLApplyDDLAlterColumnPreservesAutoIncrement proves a type-only edit
// does not drop AUTO_INCREMENT, which lives in information_schema's extra
// column and is therefore invisible to a redeclaration that does not read it.
func TestMySQLApplyDDLAlterColumnPreservesAutoIncrement(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() { dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_attr_auto") })

	mustExecMySQL(t, d, `CREATE TABLE ddl_attr_auto (id INT NOT NULL AUTO_INCREMENT PRIMARY KEY)`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_attr_auto"}

	newType := "bigint"
	if err := d.ApplyDDL(context.Background(), ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "id",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	definition := mysqlShowColumnDefinition(t, d, "ddl_attr_auto", "id")
	if !mysqlContainsFold(definition, "AUTO_INCREMENT") {
		t.Errorf("AUTO_INCREMENT was dropped: %s", definition)
	}
	if !mysqlContainsFold(definition, "bigint") {
		t.Errorf("expected the type to change to bigint: %s", definition)
	}
}

// TestMySQLApplyDDLAlterColumnPreservesCommentAndCollation proves a type-only
// edit does not drop COMMENT or an explicit COLLATE.
func TestMySQLApplyDDLAlterColumnPreservesCommentAndCollation(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() { dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_attr_meta") })

	mustExecMySQL(t, d, `CREATE TABLE ddl_attr_meta (
		id INT PRIMARY KEY,
		label VARCHAR(50) COLLATE utf8mb4_bin NOT NULL DEFAULT 'unnamed' COMMENT 'the label'
	)`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_attr_meta"}

	newType := "varchar(100)"
	if err := d.ApplyDDL(context.Background(), ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "label",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	definition := mysqlShowColumnDefinition(t, d, "ddl_attr_meta", "label")
	if !mysqlContainsFold(definition, "COMMENT 'the label'") {
		t.Errorf("COMMENT was dropped: %s", definition)
	}
	if !mysqlContainsFold(definition, "utf8mb4_bin") {
		t.Errorf("COLLATE was dropped: %s", definition)
	}
	if !mysqlContainsFold(definition, "DEFAULT 'unnamed'") {
		t.Errorf("DEFAULT was dropped: %s", definition)
	}
	if !mysqlContainsFold(definition, "varchar(100)") {
		t.Errorf("expected the type to change to varchar(100): %s", definition)
	}
}

// TestMySQLApplyDDLAlterColumnPreservesOnUpdate proves a type-only edit does
// not drop ON UPDATE CURRENT_TIMESTAMP, which MySQL packs into extra next to
// the DEFAULT_GENERATED marker.
func TestMySQLApplyDDLAlterColumnPreservesOnUpdate(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() { dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_attr_touch") })

	mustExecMySQL(t, d, `CREATE TABLE ddl_attr_touch (
		id INT PRIMARY KEY,
		seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_attr_touch"}

	nullable := true
	if err := d.ApplyDDL(context.Background(), ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "seen",
		Changes: &ddl.ColumnChanges{Nullable: &nullable},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	definition := mysqlShowColumnDefinition(t, d, "ddl_attr_touch", "seen")
	if !mysqlContainsFold(definition, "ON UPDATE CURRENT_TIMESTAMP") {
		t.Errorf("ON UPDATE was dropped: %s", definition)
	}
	if !mysqlContainsFold(definition, "DEFAULT CURRENT_TIMESTAMP") {
		t.Errorf("DEFAULT was dropped: %s", definition)
	}
}

// TestMySQLApplyDDLAlterColumnPreservesQuotedDefault proves a string default
// containing an embedded single quote survives the re-quoting round trip
// without corrupting the stored value.
func TestMySQLApplyDDLAlterColumnPreservesQuotedDefault(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() { dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_attr_quote") })

	mustExecMySQL(t, d, `CREATE TABLE ddl_attr_quote (
		id INT PRIMARY KEY,
		note VARCHAR(50) NOT NULL DEFAULT 'it''s here'
	)`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_attr_quote"}

	newType := "varchar(120)"
	if err := d.ApplyDDL(context.Background(), ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "note",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	var got string
	if err := d.DB().QueryRowContext(context.Background(), `
SELECT column_default FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'ddl_attr_quote' AND column_name = 'note'`).Scan(&got); err != nil {
		t.Fatalf("read back default: %v", err)
	}
	if got != "it's here" {
		t.Errorf("default round trip corrupted the value: got %q, want %q", got, "it's here")
	}
}

// TestMySQLApplyDDLAlterColumnPreservesGeneratedExpression proves a type-only
// edit does not drop a GENERATED ALWAYS AS (...) expression, which
// information_schema.columns reports outside extra (in
// generation_expression) and which MODIFY COLUMN otherwise silently strips,
// turning the column into an ordinary stored one.
func TestMySQLApplyDDLAlterColumnPreservesGeneratedExpression(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() { dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_attr_generated") })

	mustExecMySQL(t, d, `CREATE TABLE ddl_attr_generated (
		id INT PRIMARY KEY,
		price INT,
		tax INT GENERATED ALWAYS AS (price * 2) STORED,
		doubled INT GENERATED ALWAYS AS (price + price) VIRTUAL
	)`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_attr_generated"}

	newType := "bigint"
	if err := d.ApplyDDL(context.Background(), ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "tax",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter stored generated column: %v", err)
	}
	storedDefinition := mysqlShowColumnDefinition(t, d, "ddl_attr_generated", "tax")
	if !mysqlContainsFold(storedDefinition, "GENERATED ALWAYS AS") || !mysqlContainsFold(storedDefinition, "STORED") {
		t.Errorf("STORED generation expression was dropped: %s", storedDefinition)
	}
	if !mysqlContainsFold(storedDefinition, "bigint") {
		t.Errorf("expected the type to change to bigint: %s", storedDefinition)
	}

	nullable := true
	if err := d.ApplyDDL(context.Background(), ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "doubled",
		Changes: &ddl.ColumnChanges{Nullable: &nullable},
	}); err != nil {
		t.Fatalf("alter virtual generated column: %v", err)
	}
	virtualDefinition := mysqlShowColumnDefinition(t, d, "ddl_attr_generated", "doubled")
	if !mysqlContainsFold(virtualDefinition, "GENERATED ALWAYS AS") || !mysqlContainsFold(virtualDefinition, "VIRTUAL") {
		t.Errorf("VIRTUAL generation expression was dropped: %s", virtualDefinition)
	}
}

// mysqlContainsFold reports whether haystack contains needle, ignoring case,
// so assertions can be written in the canonical SQL spelling regardless of
// how the server renders keywords in SHOW CREATE TABLE.
func mysqlContainsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
