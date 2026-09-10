package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
)

func TestValidatePostgresDefaultsRejectsMultiStatement(t *testing.T) {
	value := "0); DROP TABLE widgets; --"
	req := ddl.Request{Operation: ddl.OperationAddColumn, Column: &ddl.ColumnDefinition{Default: &value}}
	if err := validatePostgresDefaults(req); err == nil {
		t.Error("expected rejection of a multi-statement default")
	}
}

func TestValidatePostgresDefaultsAcceptsExpression(t *testing.T) {
	value := "now()"
	req := ddl.Request{Operation: ddl.OperationAddColumn, Column: &ddl.ColumnDefinition{Default: &value}}
	if err := validatePostgresDefaults(req); err != nil {
		t.Errorf("expected acceptance, got %v", err)
	}
}
