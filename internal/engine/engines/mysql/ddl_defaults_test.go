package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
)

func TestValidateMySQLDefaultsRejectsMultiStatement(t *testing.T) {
	value := "0); DROP TABLE widgets; --"
	req := ddl.Request{Operation: ddl.OperationAddColumn, Column: &ddl.ColumnDefinition{Default: &value}}
	if err := validateMySQLDefaults(req); err == nil {
		t.Error("expected rejection of a multi-statement default")
	}
}

func TestValidateMySQLDefaultsAcceptsExpression(t *testing.T) {
	value := "CURRENT_TIMESTAMP"
	req := ddl.Request{Operation: ddl.OperationAddColumn, Column: &ddl.ColumnDefinition{Default: &value}}
	if err := validateMySQLDefaults(req); err != nil {
		t.Errorf("expected acceptance, got %v", err)
	}
}
