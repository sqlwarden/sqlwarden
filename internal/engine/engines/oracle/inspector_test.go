package oracle

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

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
	appendSource(obj, "DDL", "create table t")
	if len(obj.Descriptors) != 1 || obj.Descriptors[0].Kind != "source" || !strings.Contains(obj.Descriptors[0].Source.Body, "create table") {
		t.Errorf("descriptors = %+v", obj.Descriptors)
	}
}
