package sqlquery

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/driver"
)

func TestProviderForReturnsDefaultProviderForRegisteredDialects(t *testing.T) {
	for _, dialect := range []driver.Dialect{driver.DialectPostgres, driver.DialectMySQL, driver.DialectSQLite} {
		t.Run(string(dialect), func(t *testing.T) {
			provider := ProviderFor(dialect)
			if provider == nil {
				t.Fatal("ProviderFor() returned nil")
			}
			if provider.Classifier() == nil {
				t.Fatal("default provider classifier is nil")
			}
		})
	}
}

func TestProviderForFallsBackForUnknownDialect(t *testing.T) {
	got, err := Classify(context.Background(), ClassifyRequest{
		RequestMetadata: RequestMetadata{Dialect: driver.Dialect("future-db")},
		SQL:             "SELECT 1",
	})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if got.Kind != KindDQL {
		t.Fatalf("kind = %q, want %q", got.Kind, KindDQL)
	}
}

func TestUnsupportedCapabilitiesReturnTypedError(t *testing.T) {
	ctx := context.Background()
	meta := RequestMetadata{Dialect: driver.DialectPostgres}

	if _, err := Parse(ctx, ParseRequest{RequestMetadata: meta, SQL: "SELECT 1"}); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("Parse() error = %v, want ErrUnsupportedCapability", err)
	}
	if _, err := Complete(ctx, CompletionRequest{RequestMetadata: meta, SQL: "SELECT ", CursorOffset: 7}); !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("Complete() error = %v, want ErrUnsupportedCapability", err)
	}
	rewrite, err := Rewrite(ctx, RewriteRequest{RequestMetadata: meta, SQL: "SELECT 1", Purpose: RewritePurposePagination, Limit: 50})
	if !errors.Is(err, ErrUnsupportedCapability) {
		t.Fatalf("Rewrite() error = %v, want ErrUnsupportedCapability", err)
	}
	if rewrite.SQL != "SELECT 1" || rewrite.Applied {
		t.Fatalf("Rewrite() = %+v, want original SQL with Applied=false", rewrite)
	}
}
