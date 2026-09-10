package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestMySQLDDLSQL(t *testing.T) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "tenant`one"})
	table := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "orders"}
	tests := []struct {
		name string
		req  ddl.Request
		want string
	}{
		{name: "create table", req: ddl.Request{Operation: ddl.OperationCreateTable, Scope: scope, Name: "events", Columns: []ddl.ColumnDefinition{{Name: "id", DataType: "INT", PrimaryKey: true}}}, want: "CREATE TABLE `tenant``one`.`events` (`id` int NOT NULL, PRIMARY KEY (`id`))"},
		{name: "drop database", req: ddl.Request{Operation: ddl.OperationDropScope, Scope: scope}, want: "DROP DATABASE `tenant``one`"},
		{name: "rename column", req: ddl.Request{Operation: ddl.OperationRenameColumn, Ref: &table, Name: "old", NewName: "new`name"}, want: "ALTER TABLE `tenant``one`.`orders` RENAME COLUMN `old` TO `new``name`"},
		{name: "drop index", req: ddl.Request{Operation: ddl.OperationDropIndex, Ref: &table, Name: "orders_idx"}, want: "ALTER TABLE `tenant``one`.`orders` DROP INDEX `orders_idx`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mysqlDDLSQL(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("SQL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMySQLDDLSpecAdvertisesEditOperations(t *testing.T) {
	spec := (&Driver{}).DDLSpec()
	for _, op := range []ddl.Operation{ddl.OperationAddColumn, ddl.OperationAlterColumn, ddl.OperationCreateIndex} {
		if !spec.Supports(op) {
			t.Errorf("expected spec to support %q", op)
		}
	}
}

func TestMySQLDDLSQLAddColumn(t *testing.T) {
	req := ddl.Request{
		Operation: ddl.OperationAddColumn,
		Ref:       &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "shop"}), Kind: "table", Name: "widgets"},
		Column:    &ddl.ColumnDefinition{Name: "count", DataType: "int", Nullable: false},
	}
	sql, err := mysqlDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `shop`.`widgets` ADD COLUMN `count` int NOT NULL"
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestMySQLDDLSQLCreateIndex(t *testing.T) {
	req := ddl.Request{
		Operation:    ddl.OperationCreateIndex,
		Ref:          &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "shop"}), Kind: "table", Name: "widgets"},
		Name:         "widgets_label_idx",
		Unique:       true,
		IndexColumns: []ddl.IndexColumn{{Name: "label"}, {Name: "created_at", Descending: true}},
	}
	sql, err := mysqlDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `shop`.`widgets` ADD UNIQUE INDEX `widgets_label_idx` (`label`, `created_at` DESC)"
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestMySQLAlterColumnSQLMergesCurrentDefinition(t *testing.T) {
	current := mysqlColumnState{
		ColumnDefinition: ddl.ColumnDefinition{Name: "label", DataType: "varchar(100)", Nullable: true, Default: nil},
	}
	newType := "varchar(200)"
	sql := mysqlAlterColumnSQL(
		ddl.Request{
			Ref:     &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "shop"}), Kind: "table", Name: "widgets"},
			Name:    "label",
			Changes: &ddl.ColumnChanges{DataType: &newType},
		},
		current,
	)
	want := "ALTER TABLE `shop`.`widgets` MODIFY COLUMN `label` varchar(200) NULL"
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

// TestMySQLAlterColumnSQLPreservesMySQLOnlyAttributes proves the MODIFY
// COLUMN redeclaration carries over the attributes that live only in
// information_schema's extra/column_comment/collation_name columns. MODIFY
// COLUMN replaces the entire definition, so omitting any of them silently
// drops it from the live column.
func TestMySQLAlterColumnSQLPreservesMySQLOnlyAttributes(t *testing.T) {
	def := "CURRENT_TIMESTAMP"
	current := mysqlColumnState{
		ColumnDefinition: ddl.ColumnDefinition{Name: "seen", DataType: "datetime", Nullable: false, Default: &def},
		Comment:          "when it happened",
		OnUpdate:         "CURRENT_TIMESTAMP",
	}
	newType := "timestamp"
	got := mysqlAlterColumnSQL(
		ddl.Request{
			Ref:     &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "shop"}), Kind: "table", Name: "events"},
			Name:    "seen",
			Changes: &ddl.ColumnChanges{DataType: &newType},
		},
		current,
	)
	want := "ALTER TABLE `shop`.`events` MODIFY COLUMN `seen` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT 'when it happened'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestMySQLModifyColumnDefinition(t *testing.T) {
	label := "'unnamed'"
	tests := []struct {
		name   string
		column mysqlColumnState
		want   string
	}{
		{
			name: "auto increment key",
			column: mysqlColumnState{
				ColumnDefinition: ddl.ColumnDefinition{Name: "id", DataType: "int", Nullable: false},
				AutoIncrement:    true,
			},
			want: "`id` int NOT NULL AUTO_INCREMENT",
		},
		{
			name: "collation and comment",
			column: mysqlColumnState{
				ColumnDefinition: ddl.ColumnDefinition{Name: "label", DataType: "varchar(50)", Nullable: false, Default: &label},
				Collation:        "utf8mb4_bin",
				Comment:          "it's a label",
			},
			want: "`label` varchar(50) COLLATE utf8mb4_bin NOT NULL DEFAULT 'unnamed' COMMENT 'it''s a label'",
		},
		{
			name: "collation dropped when the new type cannot carry one",
			column: mysqlColumnState{
				ColumnDefinition: ddl.ColumnDefinition{Name: "label", DataType: "int", Nullable: true},
				Collation:        "utf8mb4_bin",
			},
			want: "`label` int NULL",
		},
		{
			name: "on update dropped when the new type is not temporal",
			column: mysqlColumnState{
				ColumnDefinition: ddl.ColumnDefinition{Name: "seen", DataType: "bigint", Nullable: true},
				OnUpdate:         "CURRENT_TIMESTAMP",
			},
			want: "`seen` bigint NULL",
		},
		{
			name: "generated stored column re-emits its expression",
			column: mysqlColumnState{
				ColumnDefinition:     ddl.ColumnDefinition{Name: "tax", DataType: "bigint", Nullable: true},
				Generated:            true,
				GenerationExpression: "(`price` * 2)",
				GenerationStored:     true,
			},
			want: "`tax` bigint GENERATED ALWAYS AS ((`price` * 2)) STORED NULL",
		},
		{
			name: "generated virtual column defaults to VIRTUAL and drops default/auto_increment/on update",
			column: mysqlColumnState{
				ColumnDefinition:     ddl.ColumnDefinition{Name: "vv", DataType: "int", Nullable: true, Default: &label},
				Generated:            true,
				GenerationExpression: "`price` + 1",
				AutoIncrement:        true,
				OnUpdate:             "CURRENT_TIMESTAMP",
			},
			want: "`vv` int GENERATED ALWAYS AS (`price` + 1) VIRTUAL NULL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mysqlModifyColumnDefinition(tt.column); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMySQLParseColumnExtra(t *testing.T) {
	tests := []struct {
		extra         string
		autoIncrement bool
		onUpdate      string
	}{
		{extra: "", autoIncrement: false, onUpdate: ""},
		{extra: "auto_increment", autoIncrement: true, onUpdate: ""},
		{extra: "DEFAULT_GENERATED on update CURRENT_TIMESTAMP", autoIncrement: false, onUpdate: "CURRENT_TIMESTAMP"},
		{extra: "on update CURRENT_TIMESTAMP(3)", autoIncrement: false, onUpdate: "CURRENT_TIMESTAMP(3)"},
		// MariaDB lowercases and parenthesizes the function name.
		{extra: "on update current_timestamp()", autoIncrement: false, onUpdate: "current_timestamp()"},
	}
	for _, tt := range tests {
		t.Run(tt.extra, func(t *testing.T) {
			autoIncrement, onUpdate := mysqlParseColumnExtra(tt.extra)
			if autoIncrement != tt.autoIncrement || onUpdate != tt.onUpdate {
				t.Errorf("got (%v, %q), want (%v, %q)", autoIncrement, onUpdate, tt.autoIncrement, tt.onUpdate)
			}
		})
	}
}

func TestMySQLRequoteDefault(t *testing.T) {
	tests := []struct {
		name string
		def  ColumnDefault
		want string
	}{
		{
			name: "string literal is quoted",
			def:  ColumnDefault{ColumnType: "varchar(50)", Raw: "unnamed"},
			want: "'unnamed'",
		},
		{
			name: "embedded quote is doubled rather than backslash escaped",
			def:  ColumnDefault{ColumnType: "varchar(50)", Raw: "it's here"},
			want: "'it''s here'",
		},
		{
			name: "backslash is doubled under the default sql_mode",
			def:  ColumnDefault{ColumnType: "varchar(50)", Raw: `c:\tmp`},
			want: `'c:\\tmp'`,
		},
		{
			name: "backslash is left alone under NO_BACKSLASH_ESCAPES",
			def:  ColumnDefault{ColumnType: "varchar(50)", Raw: `c:\tmp`, NoBackslashEscapes: true},
			want: `'c:\tmp'`,
		},
		{
			name: "expression default is left untouched",
			def:  ColumnDefault{ColumnType: "datetime", Extra: "DEFAULT_GENERATED", Raw: "CURRENT_TIMESTAMP"},
			want: "CURRENT_TIMESTAMP",
		},
		{
			// MySQL and MariaDB both report a bit default as an already-valid
			// b'..' literal, so requoting it would corrupt the value.
			name: "bit literal is left untouched",
			def:  ColumnDefault{ColumnType: "bit(8)", Raw: "b'101'"},
			want: "b'101'",
		},
		{
			name: "numeric default is left untouched",
			def:  ColumnDefault{ColumnType: "int", Raw: "7"},
			want: "7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (DefaultDecoder{}).DecodeColumnDefault(tt.def); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
