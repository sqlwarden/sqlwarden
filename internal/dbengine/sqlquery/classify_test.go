package sqlquery

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/driver"
)

func TestHeuristicClassifierClassifiesStatementKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sql  string
		want Kind
	}{
		{name: "select", sql: "SELECT * FROM users", want: KindDQL},
		{name: "with", sql: "WITH recent AS (SELECT 1) SELECT * FROM recent", want: KindDQL},
		{name: "show", sql: "SHOW TABLES", want: KindDQL},
		{name: "describe", sql: "DESCRIBE users", want: KindDQL},
		{name: "explain", sql: "EXPLAIN SELECT * FROM users", want: KindDQL},
		{name: "insert", sql: "INSERT INTO users (id) VALUES (1)", want: KindDML},
		{name: "update", sql: "UPDATE users SET name = 'a'", want: KindDML},
		{name: "delete", sql: "DELETE FROM users", want: KindDML},
		{name: "merge", sql: "MERGE INTO users USING incoming ON users.id = incoming.id", want: KindDML},
		{name: "upsert", sql: "UPSERT INTO users VALUES (1)", want: KindDML},
		{name: "create", sql: "CREATE TABLE users (id int)", want: KindDDL},
		{name: "alter", sql: "ALTER TABLE users ADD COLUMN name text", want: KindDDL},
		{name: "drop", sql: "DROP TABLE users", want: KindDDL},
		{name: "truncate", sql: "TRUNCATE TABLE users", want: KindDDL},
		{name: "rename", sql: "RENAME TABLE users TO accounts", want: KindDDL},
		{name: "empty", sql: "   ", want: KindUnknown},
		{name: "unknown", sql: "VACUUM", want: KindUnknown},
	}

	classifier := NewHeuristicClassifier()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := classifier.Classify(context.Background(), ClassifyRequest{
				RequestMetadata: RequestMetadata{Dialect: driver.DialectPostgres},
				SQL:             tc.sql,
			})
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			if got.Kind != tc.want {
				t.Fatalf("Classify(%q) kind = %q, want %q", tc.sql, got.Kind, tc.want)
			}
			if got.Source != "heuristic" {
				t.Fatalf("Classify(%q) source = %q, want heuristic", tc.sql, got.Source)
			}
		})
	}
}

func TestHeuristicClassifierPreservesLegacyContainsBehavior(t *testing.T) {
	t.Parallel()

	got, err := NewHeuristicClassifier().Classify(context.Background(), ClassifyRequest{
		RequestMetadata: RequestMetadata{Dialect: driver.DialectMySQL},
		SQL:             "/* migration */ SELECT * FROM users",
	})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if got.Kind != KindDQL {
		t.Fatalf("kind = %q, want %q", got.Kind, KindDQL)
	}
}
