package gosqlx

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/driver"
)

// TestClassifierPropertyDangerousNeverDQL generates dangerous statements,
// optionally prefixed by a benign read, across every supported dialect, and
// asserts the GoSQLX classifier never labels them DQL.
func TestClassifierPropertyDangerousNeverDQL(t *testing.T) {
	t.Parallel()

	dangerous := []string{
		"DROP TABLE accounts",
		"DELETE FROM accounts WHERE id = 1",
		"UPDATE accounts SET admin = true WHERE id = 1",
		"INSERT INTO accounts (id) VALUES (1)",
		"TRUNCATE TABLE accounts",
		"ALTER TABLE accounts ADD COLUMN c int",
		"CREATE TABLE evil (id int)",
	}
	// Each prefix is itself a benign read; the danger is the trailing statement.
	prefixes := []string{"", "SELECT 1; ", "SELECT * FROM accounts; ", "WITH t AS (SELECT 1) SELECT * FROM t; "}
	dialects := []driver.Dialect{driver.DialectPostgres, driver.DialectMySQL, driver.DialectSQLite}

	for _, d := range dialects {
		for _, prefix := range prefixes {
			for _, danger := range dangerous {
				sql := prefix + danger
				got, err := Classify(context.Background(), d, classifier.Request{SQL: sql})
				if err != nil {
					t.Fatalf("Classify(%q) error: %v", sql, err)
				}
				if got.Kind == classifier.KindDQL {
					t.Fatalf("dangerous SQL classified DQL: dialect=%s sql=%q", d, sql)
				}
			}
		}
	}
}
