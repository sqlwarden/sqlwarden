package gosqlx

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/driver"
)

func TestProviderClassifiesSQLWardenCorpus(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		dialect driver.Dialect
		sql     string
		want    classifier.Kind
	}{
		{name: "postgres select", dialect: driver.DialectPostgres, sql: "SELECT * FROM users", want: classifier.KindDQL},
		{name: "postgres cte select", dialect: driver.DialectPostgres, sql: "WITH recent AS (SELECT 1) SELECT * FROM recent", want: classifier.KindDQL},
		{name: "postgres distinct on", dialect: driver.DialectPostgres, sql: "SELECT DISTINCT ON (account_id) account_id, created_at FROM sessions ORDER BY account_id, created_at DESC", want: classifier.KindDQL},
		{name: "postgres ilike", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts WHERE email ILIKE '%@example.com'", want: classifier.KindDQL},
		{name: "postgres json operator", dialect: driver.DialectPostgres, sql: "SELECT data->>'name' FROM profiles WHERE data @> '{\"active\": true}'", want: classifier.KindDQL},
		{name: "postgres insert returning", dialect: driver.DialectPostgres, sql: "INSERT INTO accounts (email) VALUES ('a@example.com') RETURNING id", want: classifier.KindDML},
		{name: "postgres update returning", dialect: driver.DialectPostgres, sql: "UPDATE accounts SET name = 'A' WHERE id = 1 RETURNING id", want: classifier.KindDML},
		{name: "postgres delete", dialect: driver.DialectPostgres, sql: "DELETE FROM accounts WHERE id = 1", want: classifier.KindDML},
		{name: "postgres create table", dialect: driver.DialectPostgres, sql: "CREATE TABLE accounts (id BIGSERIAL PRIMARY KEY, email TEXT NOT NULL)", want: classifier.KindDDL},
		{name: "postgres alter table", dialect: driver.DialectPostgres, sql: "ALTER TABLE accounts ADD COLUMN name TEXT", want: classifier.KindDDL},
		{name: "postgres drop table", dialect: driver.DialectPostgres, sql: "DROP TABLE accounts", want: classifier.KindDDL},
		{name: "postgres truncate", dialect: driver.DialectPostgres, sql: "TRUNCATE TABLE accounts", want: classifier.KindDDL},
		{name: "postgres create materialized view", dialect: driver.DialectPostgres, sql: "CREATE MATERIALIZED VIEW active_accounts AS SELECT * FROM accounts WHERE active = true", want: classifier.KindDDL},
		{name: "postgres unsupported create sequence is unknown", dialect: driver.DialectPostgres, sql: "CREATE SEQUENCE account_id_seq", want: classifier.KindUnknown},

		{name: "mysql select backticks", dialect: driver.DialectMySQL, sql: "SELECT `id`, `email` FROM `accounts`", want: classifier.KindDQL},
		{name: "mysql show tables", dialect: driver.DialectMySQL, sql: "SHOW TABLES", want: classifier.KindDQL},
		{name: "mysql describe", dialect: driver.DialectMySQL, sql: "DESCRIBE accounts", want: classifier.KindDQL},
		{name: "mysql insert duplicate key", dialect: driver.DialectMySQL, sql: "INSERT INTO accounts (id, email) VALUES (1, 'a@example.com') ON DUPLICATE KEY UPDATE email = VALUES(email)", want: classifier.KindDML},
		{name: "mysql replace", dialect: driver.DialectMySQL, sql: "REPLACE INTO accounts (id, email) VALUES (1, 'a@example.com')", want: classifier.KindDML},
		{name: "mysql update", dialect: driver.DialectMySQL, sql: "UPDATE accounts SET email = 'b@example.com' WHERE id = 1", want: classifier.KindDML},
		{name: "mysql create table", dialect: driver.DialectMySQL, sql: "CREATE TABLE accounts (id INT AUTO_INCREMENT PRIMARY KEY, email VARCHAR(255))", want: classifier.KindDDL},
		{name: "mysql alter table", dialect: driver.DialectMySQL, sql: "ALTER TABLE accounts ADD COLUMN name VARCHAR(255)", want: classifier.KindDDL},

		{name: "sqlite select", dialect: driver.DialectSQLite, sql: "SELECT id, email FROM accounts", want: classifier.KindDQL},
		{name: "sqlite pragma", dialect: driver.DialectSQLite, sql: "PRAGMA table_info(accounts)", want: classifier.KindDQL},
		{name: "sqlite create without rowid", dialect: driver.DialectSQLite, sql: "CREATE TABLE accounts (id INTEGER PRIMARY KEY, email TEXT) WITHOUT ROWID", want: classifier.KindDDL},
		{name: "sqlite delete", dialect: driver.DialectSQLite, sql: "DELETE FROM accounts WHERE id = 1", want: classifier.KindDML},

		{name: "multi dql and dml escalates", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts; UPDATE accounts SET active = false WHERE id = 1", want: classifier.KindDML},
		{name: "multi dql and ddl escalates", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts; DROP TABLE accounts", want: classifier.KindDDL},
		{name: "multi dml and ddl escalates", dialect: driver.DialectPostgres, sql: "UPDATE accounts SET active = false WHERE id = 1; DROP TABLE accounts", want: classifier.KindDDL},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewClassifier(fixture.dialect).Classify(context.Background(), classifier.Request{SQL: fixture.sql})
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			if got.Kind != fixture.want {
				t.Fatalf("Classify() kind = %q, want %q", got.Kind, fixture.want)
			}
			if got.Source != "gosqlx" {
				t.Fatalf("Classify() source = %q, want gosqlx", got.Source)
			}
		})
	}
}

func TestProviderClassifiesInvalidSQLAsUnknown(t *testing.T) {
	t.Parallel()

	got, err := NewClassifier(driver.DialectPostgres).Classify(context.Background(), classifier.Request{SQL: "SELECT FROM WHERE"})
	if err != nil {
		t.Fatalf("Classify() error = %v", err)
	}
	if got.Kind != classifier.KindUnknown {
		t.Fatalf("kind = %q, want %q", got.Kind, classifier.KindUnknown)
	}
}

func TestProviderParsesCompleteAndIncompleteSQL(t *testing.T) {
	t.Parallel()

	complete, err := NewParser(driver.DialectPostgres).Parse(context.Background(), parser.Request{
		SQL: "SELECT * FROM accounts; SELECT * FROM sessions",
	})
	if err != nil {
		t.Fatalf("Parse() complete error = %v", err)
	}
	if !complete.Complete || complete.StatementCount != 2 || complete.AST == nil {
		t.Fatalf("complete parse = %+v, want complete with two statements and AST", complete)
	}

	cursor := len("SELECT * FROM")
	incomplete, err := NewParser(driver.DialectPostgres).Parse(context.Background(), parser.Request{
		SQL:          "SELECT * FROM",
		CursorOffset: &cursor,
	})
	if err != nil {
		t.Fatalf("Parse() incomplete error = %v", err)
	}
	if incomplete.Complete {
		t.Fatal("incomplete parse marked complete")
	}
}

func TestProviderRewritePagination(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name        string
		dialect     driver.Dialect
		sql         string
		limit       int
		offset      int
		wantApplied bool
		wantReason  string
		wantParts   []string
	}{
		{
			name:        "simple select",
			dialect:     driver.DialectPostgres,
			sql:         "SELECT * FROM accounts",
			limit:       50,
			offset:      100,
			wantApplied: true,
			wantParts:   []string{"LIMIT 50", "OFFSET 100"},
		},
		{
			name:        "existing limit offset replaced",
			dialect:     driver.DialectMySQL,
			sql:         "SELECT * FROM accounts LIMIT 10 OFFSET 20",
			limit:       25,
			offset:      75,
			wantApplied: true,
			wantParts:   []string{"LIMIT 25", "OFFSET 75"},
		},
		{
			name:        "dml not rewritten",
			dialect:     driver.DialectPostgres,
			sql:         "UPDATE accounts SET active = false WHERE id = 1",
			limit:       50,
			offset:      0,
			wantApplied: false,
			wantReason:  "only select statements can be rewritten",
		},
		{
			name:        "multi statement not rewritten",
			dialect:     driver.DialectPostgres,
			sql:         "SELECT * FROM accounts; SELECT * FROM sessions",
			limit:       50,
			offset:      0,
			wantApplied: false,
			wantReason:  "only single statements can be rewritten",
		},
		{
			name:        "invalid not rewritten",
			dialect:     driver.DialectPostgres,
			sql:         "SELECT FROM WHERE",
			limit:       50,
			offset:      0,
			wantApplied: false,
			wantReason:  "parse failed",
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewRewriter(fixture.dialect).Rewrite(context.Background(), rewriter.Request{
				SQL:     fixture.sql,
				Purpose: rewriter.PurposePagination,
				Limit:   fixture.limit,
				Offset:  fixture.offset,
			})
			if err != nil {
				t.Fatalf("Rewrite() error = %v", err)
			}
			if got.Applied != fixture.wantApplied {
				t.Fatalf("Applied = %v, want %v; result=%+v", got.Applied, fixture.wantApplied, got)
			}
			if fixture.wantReason != "" && got.Reason != fixture.wantReason {
				t.Fatalf("Reason = %q, want %q", got.Reason, fixture.wantReason)
			}
			for _, part := range fixture.wantParts {
				if !strings.Contains(got.SQL, part) {
					t.Fatalf("rewritten SQL %q does not contain %q", got.SQL, part)
				}
			}
		})
	}
}
