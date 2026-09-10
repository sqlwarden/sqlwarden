package tidb

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func tidbShowColumnDefinition(t *testing.T, d *driver, table, column string) string {
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

// TestApplyDDLAlterColumnPreservesAttributes proves TiDB round trips the
// inherited MySQL ALTER COLUMN implementation. TiDB reports
// information_schema column defaults with MySQL 8's semantics — string
// literals unquoted, expression defaults flagged DEFAULT_GENERATED in extra —
// so unlike MariaDB it needs no decoder of its own, and this test is what
// keeps that inheritance honest.
func TestApplyDDLAlterColumnPreservesAttributes(t *testing.T) {
	d := connect(t)
	ctx := context.Background()
	drop := func() { _, _ = d.DB().ExecContext(ctx, "DROP TABLE IF EXISTS ddl_tidb_attrs") }
	drop()
	t.Cleanup(drop)

	if _, err := d.DB().ExecContext(ctx, `CREATE TABLE ddl_tidb_attrs (
		id INT NOT NULL PRIMARY KEY,
		label VARCHAR(50) COLLATE utf8mb4_bin NOT NULL DEFAULT 'unnamed' COMMENT 'the label',
		seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}
	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database}),
		Kind:  "table", Name: "ddl_tidb_attrs",
	}

	labelType := "varchar(100)"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "label",
		Changes: &ddl.ColumnChanges{DataType: &labelType},
	}); err != nil {
		t.Fatalf("alter label column: %v", err)
	}
	label := tidbShowColumnDefinition(t, d, "ddl_tidb_attrs", "label")
	for _, want := range []string{"varchar(100)", "default 'unnamed'", "comment 'the label'"} {
		if !strings.Contains(strings.ToLower(label), want) {
			t.Errorf("expected %q in %s", want, label)
		}
	}
	// SHOW CREATE TABLE omits a COLLATE clause that matches the table
	// default, so the collation is asserted against information_schema.
	var collation string
	if err := d.DB().QueryRowContext(ctx, `
SELECT collation_name FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = 'ddl_tidb_attrs' AND column_name = 'label'`).Scan(&collation); err != nil {
		t.Fatalf("read back collation: %v", err)
	}
	if collation != "utf8mb4_bin" {
		t.Errorf("collation was not preserved: got %q", collation)
	}

	nullable := true
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "seen",
		Changes: &ddl.ColumnChanges{Nullable: &nullable},
	}); err != nil {
		t.Fatalf("alter seen column: %v", err)
	}
	seen := tidbShowColumnDefinition(t, d, "ddl_tidb_attrs", "seen")
	for _, want := range []string{"default current_timestamp", "on update current_timestamp"} {
		if !strings.Contains(strings.ToLower(seen), want) {
			t.Errorf("expected %q in %s", want, seen)
		}
	}
}
