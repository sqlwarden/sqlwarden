package oracle

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func i64(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func TestOracleColumnType(t *testing.T) {
	null := sql.NullInt64{}
	cases := []struct {
		name                     string
		dataType                 string
		length, precision, scale sql.NullInt64
		want                     string
	}{
		{"number no precision", "NUMBER", null, null, null, "NUMBER"},
		{"number precision only", "NUMBER", null, i64(10), null, "NUMBER(10)"},
		{"number precision and scale", "NUMBER", null, i64(10), i64(2), "NUMBER(10,2)"},
		{"varchar2 with length", "VARCHAR2", i64(100), null, null, "VARCHAR2(100)"},
		{"passthrough", "CLOB", null, null, null, "CLOB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oracleColumnType(tc.dataType, tc.length, tc.precision, tc.scale); got != tc.want {
				t.Fatalf("oracleColumnType = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOracleSchemaSpec(t *testing.T) {
	spec := (&oracleDriver{}).SchemaSpec()
	if spec.Dialect != "oracle" {
		t.Fatalf("dialect = %q", spec.Dialect)
	}
	got := map[string]metadata.SchemaObjectKind{}
	for _, k := range spec.Kinds {
		got[k.Kind] = k
	}
	for _, want := range []string{"table", "view", "materialized_view", "sequence", "function", "procedure", "package"} {
		if _, ok := got[want]; !ok {
			t.Errorf("SchemaSpec missing kind %q", want)
		}
	}
	if !got["table"].Relational || !got["table"].SupportsDiagram {
		t.Errorf("table kind flags wrong: %+v", got["table"])
	}
	if got["sequence"].Relational {
		t.Errorf("sequence must not be relational")
	}
	if !got["materialized_view"].Relational {
		t.Errorf("materialized_view must be relational")
	}
}

func TestOraclePairFilter(t *testing.T) {
	refs := []metadata.ObjectRef{
		{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "EMP"},
		{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "DEPT"},
	}
	pred, args := oraclePairFilter(refs, 1)
	if pred != "(:1,:2),(:3,:4)" {
		t.Fatalf("pred = %q", pred)
	}
	if len(args) != 4 || args[0] != "HR" || args[1] != "EMP" || args[2] != "HR" || args[3] != "DEPT" {
		t.Fatalf("args = %v", args)
	}
	predOffset, _ := oraclePairFilter(refs[:1], 5)
	if predOffset != "(:5,:6)" {
		t.Fatalf("offset pred = %q", predOffset)
	}
}

func TestOracleDictViewAndOwner(t *testing.T) {
	all := oracleDict{}
	user := oracleDict{user: true}

	if all.view("all_tab_columns") != "all_tab_columns" {
		t.Errorf("all.view = %q", all.view("all_tab_columns"))
	}
	if user.view("all_tab_columns") != "user_tab_columns" {
		t.Errorf("user.view = %q", user.view("all_tab_columns"))
	}
	if all.ownerCol("cc.owner") != "cc.owner" {
		t.Errorf("all.ownerCol = %q", all.ownerCol("cc.owner"))
	}
	if user.ownerCol("cc.owner") != "USER" {
		t.Errorf("user.ownerCol = %q", user.ownerCol("cc.owner"))
	}
	if all.ownerJoin("cc.owner", "c.owner") != "cc.owner = c.owner AND " {
		t.Errorf("all.ownerJoin = %q", all.ownerJoin("cc.owner", "c.owner"))
	}
	if user.ownerJoin("cc.owner", "c.owner") != "" {
		t.Errorf("user.ownerJoin = %q", user.ownerJoin("cc.owner", "c.owner"))
	}
}

func TestOracleDictObjFilter(t *testing.T) {
	refs := []metadata.ObjectRef{
		{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "EMP"},
		{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "DEPT"},
	}

	allPred, allArgs := oracleDict{}.objFilter("owner", "table_name", refs, 1)
	if allPred != "(owner, table_name) IN ((:1,:2),(:3,:4))" {
		t.Fatalf("all pred = %q", allPred)
	}
	if len(allArgs) != 4 || allArgs[0] != "HR" || allArgs[1] != "EMP" || allArgs[3] != "DEPT" {
		t.Fatalf("all args = %v", allArgs)
	}

	userPred, userArgs := oracleDict{user: true}.objFilter("owner", "table_name", refs, 1)
	if userPred != "table_name IN (:1,:2)" {
		t.Fatalf("user pred = %q", userPred)
	}
	if len(userArgs) != 2 || userArgs[0] != "EMP" || userArgs[1] != "DEPT" {
		t.Fatalf("user args = %v", userArgs)
	}

	offsetPred, _ := oracleDict{user: true}.objFilter("owner", "sequence_name", refs[:1], 5)
	if offsetPred != "sequence_name IN (:5)" {
		t.Fatalf("offset pred = %q", offsetPred)
	}
}

func TestOracleSystemSchemasExcludesUserSchema(t *testing.T) {
	if _, isSystem := oracleSystemSchemas["SYS"]; !isSystem {
		t.Error("SYS should be a system schema")
	}
	if _, isSystem := oracleSystemSchemas["HR"]; isSystem {
		t.Error("HR should not be a system schema")
	}
}

func TestOracleSetAttrHelpers(t *testing.T) {
	obj := &metadata.Object{}
	setObjectAttr(obj, "comment", "")
	if obj.Attributes != nil {
		t.Error("empty value must not create an attribute map")
	}
	setObjectAttr(obj, "comment", "hi")
	if obj.Attributes["comment"] != "hi" {
		t.Errorf("attributes = %v", obj.Attributes)
	}
	col := &metadata.Column{}
	setColumnAttr(col, "comment", "c")
	if col.Attributes["comment"] != "c" {
		t.Errorf("column attributes = %v", col.Attributes)
	}
	if oracleSourceDescriptor("DDL", "") != nil {
		t.Error("empty body must yield a nil descriptor")
	}
	desc := oracleSourceDescriptor("DDL", "create table t")
	if desc == nil || desc.Kind != "source" || desc.Title != "DDL" || !strings.Contains(desc.Source.Body, "create table") {
		t.Errorf("descriptor = %+v", desc)
	}
}
