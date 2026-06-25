package gosqlx

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/rewriter"
)

func TestProviderSecurityClassificationNeverTreatsDangerousSQLAsDQL(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		dialect dbengine.Dialect
		sql     string
	}{
		{name: "generic select then delete", dialect: dbengine.DialectPostgres, sql: "SELECT * FROM accounts; DELETE FROM accounts WHERE id = 1"},
		{name: "generic select then drop", dialect: dbengine.DialectPostgres, sql: "SELECT * FROM accounts; DROP TABLE accounts"},
		{name: "generic select then update", dialect: dbengine.DialectPostgres, sql: "SELECT * FROM accounts WHERE id = 1; UPDATE accounts SET admin = true WHERE id = 1"},
		{name: "generic invalid comment-obfuscated select", dialect: dbengine.DialectPostgres, sql: "SEL/**/ECT * FROM accounts"},
		{name: "generic invalid comment-obfuscated update", dialect: dbengine.DialectPostgres, sql: "UPDATE/**/accounts SET active = false"},
		{name: "generic invalid comment-obfuscated drop", dialect: dbengine.DialectPostgres, sql: "DROP/**/TABLE accounts"},
		{name: "generic transaction wraps mutation", dialect: dbengine.DialectPostgres, sql: "BEGIN; UPDATE accounts SET admin = true WHERE id = 1; COMMIT"},
		{name: "generic set role then select", dialect: dbengine.DialectPostgres, sql: "SET ROLE admin; SELECT * FROM accounts"},
		{name: "generic call procedure", dialect: dbengine.DialectPostgres, sql: "CALL dangerous_proc()"},

		{name: "postgres data modifying delete cte", dialect: dbengine.DialectPostgres, sql: "WITH deleted AS (DELETE FROM accounts WHERE id = 1 RETURNING *) SELECT * FROM deleted"},
		{name: "postgres data modifying update cte", dialect: dbengine.DialectPostgres, sql: "WITH updated AS (UPDATE accounts SET admin = true WHERE id = 1 RETURNING *) SELECT * FROM updated"},
		{name: "postgres explain delete", dialect: dbengine.DialectPostgres, sql: "EXPLAIN DELETE FROM accounts WHERE id = 1"},
		{name: "postgres explain analyze update", dialect: dbengine.DialectPostgres, sql: "EXPLAIN ANALYZE UPDATE accounts SET active = false WHERE id = 1"},
		{name: "postgres copy program", dialect: dbengine.DialectPostgres, sql: "COPY accounts TO PROGRAM 'cat > /tmp/accounts.txt'"},
		{name: "postgres do block", dialect: dbengine.DialectPostgres, sql: "DO $$ BEGIN DELETE FROM accounts; END $$"},
		{name: "postgres select into table", dialect: dbengine.DialectPostgres, sql: "SELECT * INTO copied_accounts FROM accounts"},
		{name: "postgres notify", dialect: dbengine.DialectPostgres, sql: "NOTIFY account_changed, '1'"},
		{name: "postgres listen", dialect: dbengine.DialectPostgres, sql: "LISTEN account_changed"},

		{name: "mysql load data infile", dialect: dbengine.DialectMySQL, sql: "LOAD DATA INFILE '/tmp/accounts.csv' INTO TABLE accounts"},
		{name: "mysql select into outfile", dialect: dbengine.DialectMySQL, sql: "SELECT * FROM accounts INTO OUTFILE '/tmp/accounts.csv'"},
		{name: "mysql create temporary table", dialect: dbengine.DialectMySQL, sql: "CREATE TEMPORARY TABLE copied_accounts AS SELECT * FROM accounts"},
		{name: "mysql lock tables", dialect: dbengine.DialectMySQL, sql: "LOCK TABLES accounts WRITE"},
		{name: "mysql call procedure", dialect: dbengine.DialectMySQL, sql: "CALL dangerous_proc()"},
		{name: "mysql versioned comment mutation", dialect: dbengine.DialectMySQL, sql: "SELECT 1 /*!50000; DROP TABLE accounts */"},

		{name: "sqlite attach database", dialect: dbengine.DialectSQLite, sql: "ATTACH DATABASE '/tmp/other.db' AS other"},
		{name: "sqlite detach database", dialect: dbengine.DialectSQLite, sql: "DETACH DATABASE other"},
		{name: "sqlite writable schema pragma", dialect: dbengine.DialectSQLite, sql: "PRAGMA writable_schema = ON"},
		{name: "sqlite vacuum", dialect: dbengine.DialectSQLite, sql: "VACUUM"},
		{name: "sqlite reindex", dialect: dbengine.DialectSQLite, sql: "REINDEX"},
		{name: "sqlite create temp table", dialect: dbengine.DialectSQLite, sql: "CREATE TEMP TABLE copied_accounts AS SELECT * FROM accounts"},
		{name: "sqlite insert or replace", dialect: dbengine.DialectSQLite, sql: "INSERT OR REPLACE INTO accounts (id, email) VALUES (1, 'a@example.com')"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := Classify(context.Background(), fixture.dialect, classifier.Request{SQL: fixture.sql})
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
		dialect dbengine.Dialect
		sql     string
	}{
		{name: "semicolon in string", dialect: dbengine.DialectPostgres, sql: "SELECT '; DELETE FROM accounts' AS text"},
		{name: "drop in string", dialect: dbengine.DialectPostgres, sql: "SELECT * FROM accounts WHERE email = 'a; DROP TABLE accounts'"},
		{name: "drop in block comment", dialect: dbengine.DialectPostgres, sql: "SELECT 1 /* ; DROP TABLE accounts */"},
		{name: "drop in line comment", dialect: dbengine.DialectPostgres, sql: "SELECT 1 -- ; DROP TABLE accounts"},
		{name: "mysql drop in string", dialect: dbengine.DialectMySQL, sql: "SELECT '; DROP TABLE accounts' AS text"},
		{name: "sqlite drop in string", dialect: dbengine.DialectSQLite, sql: "SELECT '; DROP TABLE accounts' AS text"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := Classify(context.Background(), fixture.dialect, classifier.Request{SQL: fixture.sql})
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
		dialect dbengine.Dialect
		sql     string
	}{
		{name: "multi statement injection", dialect: dbengine.DialectPostgres, sql: "SELECT * FROM accounts; DROP TABLE accounts"},
		{name: "postgres data modifying cte", dialect: dbengine.DialectPostgres, sql: "WITH deleted AS (DELETE FROM accounts RETURNING *) SELECT * FROM deleted"},
		{name: "postgres select for update", dialect: dbengine.DialectPostgres, sql: "SELECT * FROM accounts FOR UPDATE"},
		{name: "postgres select into table", dialect: dbengine.DialectPostgres, sql: "SELECT * INTO copied_accounts FROM accounts"},
		{name: "mysql select into outfile", dialect: dbengine.DialectMySQL, sql: "SELECT * FROM accounts INTO OUTFILE '/tmp/accounts.csv'"},
		{name: "sqlite writable pragma", dialect: dbengine.DialectSQLite, sql: "PRAGMA writable_schema = ON"},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			got, err := Rewrite(context.Background(), fixture.dialect, rewriter.Request{
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
