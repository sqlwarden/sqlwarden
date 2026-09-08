package sqlserver

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/safety"
)

func TestCheckFlagsMissingWhere(t *testing.T) {
	d := &Driver{}
	res, err := d.Check(context.Background(), safety.Request{SQL: "UPDATE dbo.t SET a = 1; DELETE FROM dbo.t;"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Unsafe || len(res.Statements) != 2 {
		t.Fatalf("want 2 unsafe statements, got %+v", res)
	}
}

func TestCheckAllowsWhere(t *testing.T) {
	d := &Driver{}
	res, err := d.Check(context.Background(), safety.Request{SQL: "UPDATE dbo.t SET a = 1 WHERE id = 1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Unsafe {
		t.Fatalf("want safe, got %+v", res)
	}
}
