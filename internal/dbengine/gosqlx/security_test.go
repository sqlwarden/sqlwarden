package gosqlx

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/driver"
)

func TestProviderSecurityClassificationNeverTreatsDangerousSQLAsDQL(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		dialect driver.Dialect
		sql     string
	}{
		{name: "generic select then delete", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts; DELETE FROM accounts WHERE id = 1"},
		{name: "generic select then drop", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts; DROP TABLE accounts"},
		{name: "generic select then update", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts WHERE id = 1; UPDATE accounts SET admin = true WHERE id = 1"},
		{name: "generic invalid comment-obfuscated select", dialect: driver.DialectPostgres, sql: "SEL/**/ECT * FROM accounts"},
		{name: "generic invalid comment-obfuscated update", dialect: driver.DialectPostgres, sql: "UPDATE/**/accounts SET active = false"},
		{name: "generic invalid comment-obfuscated drop", dialect: driver.DialectPostgres, sql: "DROP/**/TABLE accounts"},
		{name: "generic transaction wraps mutation", dialect: driver.DialectPostgres, sql: "BEGIN; UPDATE accounts SET admin = true WHERE id = 1; COMMIT"},
		{name: "generic set role then select", dialect: driver.DialectPostgres, sql: "SET ROLE admin; SELECT * FROM accounts"},
		{name: "generic call procedure", dialect: driver.DialectPostgres, sql: "CALL dangerous_proc()"},

		{name: "postgres data modifying delete cte", dialect: driver.DialectPostgres, sql: "WITH deleted AS (DELETE FROM accounts WHERE id = 1 RETURNING *) SELECT * FROM deleted"},
		{name: "postgres data modifying update cte", dialect: driver.DialectPostgres, sql: "WITH updated AS (UPDATE accounts SET admin = true WHERE id = 1 RETURNING *) SELECT * FROM updated"},
		{name: "postgres explain delete", dialect: driver.DialectPostgres, sql: "EXPLAIN DELETE FROM accounts WHERE id = 1"},
		{name: "postgres explain analyze update", dialect: driver.DialectPostgres, sql: "EXPLAIN ANALYZE UPDATE accounts SET active = false WHERE id = 1"},
		{name: "postgres copy program", dialect: driver.DialectPostgres, sql: "COPY accounts TO PROGRAM 'cat > /tmp/accounts.txt'"},
		{name: "postgres do block", dialect: driver.DialectPostgres, sql: "DO $$ BEGIN DELETE FROM accounts; END $$"},
		{name: "postgres select into table", dialect: driver.DialectPostgres, sql: "SELECT * INTO copied_accounts FROM accounts"},
		{name: "postgres notify", dialect: driver.DialectPostgres, sql: "NOTIFY account_changed, '1'"},
		{name: "postgres listen", dialect: driver.DialectPostgres, sql: "LISTEN account_changed"},

		{name: "mysql load data infile", dialect: driver.DialectMySQL, sql: "LOAD DATA INFILE '/tmp/accounts.csv' INTO TABLE accounts"},
		{name: "mysql select into outfile", dialect: driver.DialectMySQL, sql: "SELECT * FROM accounts INTO OUTFILE '/tmp/accounts.csv'"},
		{name: "mysql create temporary table", dialect: driver.DialectMySQL, sql: "CREATE TEMPORARY TABLE copied_accounts AS SELECT * FROM accounts"},
		{name: "mysql lock tables", dialect: driver.DialectMySQL, sql: "LOCK TABLES accounts WRITE"},
		{name: "mysql call procedure", dialect: driver.DialectMySQL, sql: "CALL dangerous_proc()"},
		{name: "mysql versioned comment mutation", dialect: driver.DialectMySQL, sql: "SELECT 1 /*!50000; DROP TABLE accounts */"},

		{name: "sqlite attach database", dialect: driver.DialectSQLite, sql: "ATTACH DATABASE '/tmp/other.db' AS other"},
		{name: "sqlite detach database", dialect: driver.DialectSQLite, sql: "DETACH DATABASE other"},
		{name: "sqlite writable schema pragma", dialect: driver.DialectSQLite, sql: "PRAGMA writable_schema = ON"},
		{name: "sqlite vacuum", dialect: driver.DialectSQLite, sql: "VACUUM"},
		{name: "sqlite reindex", dialect: driver.DialectSQLite, sql: "REINDEX"},
		{name: "sqlite create temp table", dialect: driver.DialectSQLite, sql: "CREATE TEMP TABLE copied_accounts AS SELECT * FROM accounts"},
		{name: "sqlite insert or replace", dialect: driver.DialectSQLite, sql: "INSERT OR REPLACE INTO accounts (id, email) VALUES (1, 'a@example.com')"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewClassifier(fixture.dialect).Classify(context.Background(), classifier.Request{SQL: fixture.sql})
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			if got.Kind == classifier.KindDQL {
				t.Fatalf("dangerous SQL classified as DQL: sql=%q", fixture.sql)
			}
		})
	}
}

func TestProviderSecurityClassificationAllowsSafeDQLWithInjectionLikeText(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		dialect driver.Dialect
		sql     string
	}{
		{name: "semicolon in string", dialect: driver.DialectPostgres, sql: "SELECT '; DELETE FROM accounts' AS text"},
		{name: "drop in string", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts WHERE email = 'a; DROP TABLE accounts'"},
		{name: "drop in block comment", dialect: driver.DialectPostgres, sql: "SELECT 1 /* ; DROP TABLE accounts */"},
		{name: "drop in line comment", dialect: driver.DialectPostgres, sql: "SELECT 1 -- ; DROP TABLE accounts"},
		{name: "mysql drop in string", dialect: driver.DialectMySQL, sql: "SELECT '; DROP TABLE accounts' AS text"},
		{name: "sqlite drop in string", dialect: driver.DialectSQLite, sql: "SELECT '; DROP TABLE accounts' AS text"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewClassifier(fixture.dialect).Classify(context.Background(), classifier.Request{SQL: fixture.sql})
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			if got.Kind != classifier.KindDQL {
				t.Fatalf("safe DQL classified as %q, want DQL", got.Kind)
			}
		})
	}
}

func TestProviderRewriteRejectsSecuritySensitiveSQL(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		dialect driver.Dialect
		sql     string
	}{
		{name: "multi statement injection", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts; DROP TABLE accounts"},
		{name: "postgres data modifying cte", dialect: driver.DialectPostgres, sql: "WITH deleted AS (DELETE FROM accounts RETURNING *) SELECT * FROM deleted"},
		{name: "postgres select for update", dialect: driver.DialectPostgres, sql: "SELECT * FROM accounts FOR UPDATE"},
		{name: "postgres select into table", dialect: driver.DialectPostgres, sql: "SELECT * INTO copied_accounts FROM accounts"},
		{name: "mysql select into outfile", dialect: driver.DialectMySQL, sql: "SELECT * FROM accounts INTO OUTFILE '/tmp/accounts.csv'"},
		{name: "sqlite writable pragma", dialect: driver.DialectSQLite, sql: "PRAGMA writable_schema = ON"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewRewriter(fixture.dialect).Rewrite(context.Background(), rewriter.Request{
				SQL:     fixture.sql,
				Purpose: rewriter.PurposePagination,
				Limit:   50,
			})
			if err != nil {
				t.Fatalf("Rewrite() error = %v", err)
			}
			if got.Applied {
				t.Fatalf("rewrite applied to security-sensitive SQL: %q -> %q", fixture.sql, got.SQL)
			}
			if strings.TrimSpace(got.Reason) == "" {
				t.Fatalf("rewrite rejection reason is empty for %q", fixture.sql)
			}
		})
	}
}
