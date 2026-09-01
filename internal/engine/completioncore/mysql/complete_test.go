package mysql_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine/completioncore"
	"github.com/sqlwarden/internal/engine/completioncore/completiontest"
	coremysql "github.com/sqlwarden/internal/engine/completioncore/mysql"
)

func TestScopeScenarios(t *testing.T) {
	catalog := completiontest.Metadata("mysql", "sakila", "sakila")
	column := completioncore.CandidateColumn
	completiontest.Run(t, func(ctx context.Context, sql string, cursor int, metadata completioncore.MetadataResolver) ([]completioncore.Candidate, error) {
		candidates, _, err := coremysql.Complete(ctx, sql, cursor, nil, metadata)
		return candidates, err
	}, catalog, []completiontest.Scenario{
		{
			Name: "qualified join alias",
			SQL:  "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE s.|",
			Require: []completiontest.Expected{
				{Text: "id", Type: column}, {Text: "store_name", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "inventory_name", Type: column}},
		},
		{
			Name: "final alias after quoted qualified expression",
			SQL:  "SELECT * FROM film f\nJOIN film_actor fa\nWHERE f.`description` = fa.|",
			Require: []completiontest.Expected{
				{Text: "actor_id", Type: column}, {Text: "film_id", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "description", Type: column}},
		},
		{
			Name: "unqualified join includes unique and qualifies conflicts",
			SQL:  "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE |",
			Require: []completiontest.Expected{
				{Text: "i.id", Type: column}, {Text: "s.id", Type: column},
				{Text: "inventory_name", Type: column}, {Text: "store_name", Type: column},
			},
		},
		{
			Name:    "unknown qualifier does not leak",
			SQL:     "SELECT * FROM inventory i WHERE missing.|",
			Exclude: []completiontest.Expected{{Text: "inventory_name", Type: column}},
		},
		{
			Name: "explicit CTE columns",
			SQL:  "WITH picked(code, label) AS (SELECT film_id, title FROM film) SELECT * FROM picked p WHERE p.|",
			Require: []completiontest.Expected{
				{Text: "code", Type: column}, {Text: "label", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "film_id", Type: column}},
		},
		{
			Name: "inferred CTE columns",
			SQL:  "WITH picked AS (SELECT film_id, title AS label FROM film) SELECT * FROM picked p WHERE p.|",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "label", Type: column},
			},
		},
		{
			Name: "CTE name in empty relation position",
			SQL:  "WITH picked AS (SELECT film_id, title FROM film) SELECT * FROM |",
			Require: []completiontest.Expected{
				{Text: "picked", Type: completioncore.CandidateTable},
			},
		},
		{
			Name: "CTE name from partial relation prefix",
			SQL:  "WITH picked AS (SELECT film_id, title FROM film) SELECT * FROM pic|",
			Require: []completiontest.Expected{
				{Text: "picked", Type: completioncore.CandidateTable},
			},
		},
		{
			Name: "CTE name in a second comma-separated relation slot",
			SQL:  "WITH picked AS (SELECT film_id FROM film) SELECT * FROM film, |",
			Require: []completiontest.Expected{
				{Text: "picked", Type: completioncore.CandidateTable},
			},
		},
		{
			Name: "CTE name in join relation position",
			SQL:  "WITH picked AS (SELECT film_id FROM film) SELECT * FROM film JOIN |",
			Require: []completiontest.Expected{
				{Text: "picked", Type: completioncore.CandidateTable},
			},
		},
		{
			Name:    "CTE relation name is not a select expression",
			SQL:     "WITH picked AS (SELECT film_id FROM film) SELECT | FROM film",
			Exclude: []completiontest.Expected{{Text: "picked", Type: completioncore.CandidateTable}},
		},
		{
			Name: "multiple CTE names in relation position",
			SQL:  "WITH first_pick AS (SELECT film_id FROM film), second_pick AS (SELECT actor_id FROM film_actor) SELECT * FROM |",
			Require: []completiontest.Expected{
				{Text: "first_pick", Type: completioncore.CandidateTable},
				{Text: "second_pick", Type: completioncore.CandidateTable},
			},
		},
		{
			Name: "nested CTE name in its relation position",
			SQL:  "SELECT * FROM (WITH nested_pick AS (SELECT film_id FROM film) SELECT * FROM |) nested",
			Require: []completiontest.Expected{
				{Text: "nested_pick", Type: completioncore.CandidateTable},
			},
		},
		{
			Name:    "nested sibling CTE does not leak outward",
			SQL:     "SELECT * FROM (WITH nested_pick AS (SELECT film_id FROM film) SELECT * FROM nested_pick) nested JOIN |",
			Exclude: []completiontest.Expected{{Text: "nested_pick", Type: completioncore.CandidateTable}},
		},
		{
			Name: "unqualified CTE projected columns",
			SQL:  "WITH picked AS (SELECT film_id, title AS amt FROM film) SELECT | FROM picked",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "amt", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "title", Type: column}},
		},
		{
			Name: "CTE columns from a partial projection prefix before FROM",
			SQL:  "WITH picked AS (SELECT film_id, title AS amount FROM film) SELECT amo| FROM picked",
			Require: []completiontest.Expected{
				{Text: "amount", Type: column},
			},
		},
		{
			Name: "CTE columns from an empty projection slot before FROM",
			SQL:  "WITH picked AS (SELECT film_id, title AS amount FROM film) SELECT | FROM picked",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "amount", Type: column},
			},
		},
		{
			Name:    "CTE name does not cross statement boundary",
			SQL:     "WITH picked AS (SELECT film_id FROM film) SELECT * FROM picked; SELECT * FROM |",
			Exclude: []completiontest.Expected{{Text: "picked", Type: completioncore.CandidateTable}},
		},
		{
			Name: "standalone incomplete CTE body",
			SQL:  "WITH picked AS (\n  SELECT \n    |\n  FROM film\n)",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name: "standalone incomplete CTE ignores commented outer query",
			SQL:  "WITH picked AS (\n  SELECT \n    |\n  FROM film\n)\n-- SELECT *\n-- FROM picked",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name: "derived table columns",
			SQL:  "SELECT * FROM (SELECT customer_id, email AS address FROM customer) c WHERE c.|",
			Require: []completiontest.Expected{
				{Text: "customer_id", Type: column}, {Text: "address", Type: column},
			},
		},
		{
			Name: "correlated subquery sees outer alias",
			SQL:  "SELECT * FROM customer c WHERE EXISTS (SELECT 1 FROM film f WHERE c.|)",
			Require: []completiontest.Expected{
				{Text: "customer_id", Type: column}, {Text: "email", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "title", Type: column}},
		},
		{
			Name:    "non lateral derived table hides outer alias",
			SQL:     "SELECT * FROM customer c JOIN (SELECT * FROM film f WHERE c.|) x ON TRUE",
			Exclude: []completiontest.Expected{{Text: "customer_id", Type: column}, {Text: "email", Type: column}},
		},
		{
			Name: "lateral derived table sees outer alias",
			SQL:  "SELECT * FROM customer c JOIN LATERAL (SELECT c.|) x ON TRUE",
			Require: []completiontest.Expected{
				{Text: "customer_id", Type: column}, {Text: "email", Type: column},
			},
		},
		{
			Name: "statement boundary",
			SQL:  "SELECT * FROM inventory i; SELECT * FROM store s WHERE s.|",
			Require: []completiontest.Expected{
				{Text: "store_name", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "inventory_name", Type: column}},
		},
		{
			Name: "select list resolves following from",
			SQL:  "SELECT | FROM film f",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name:    "group by sees select alias",
			SQL:     "SELECT film_id, SUM(film_id) AS total_amount FROM film GROUP BY |",
			Require: []completiontest.Expected{{Text: "total_amount", Type: column}},
		},
		{
			Name:    "group by sees bare select alias",
			SQL:     "SELECT SUM(film_id) total_amount FROM film GROUP BY |",
			Require: []completiontest.Expected{{Text: "total_amount", Type: column}},
		},
		{
			Name:    "group keyword hides select alias before by",
			SQL:     "SELECT SUM(film_id) AS total_amount FROM film GROUP |",
			Exclude: []completiontest.Expected{{Text: "total_amount", Type: column}},
		},
		{
			Name:    "having sees select alias",
			SQL:     "SELECT film_id, SUM(film_id) AS total_amount FROM film GROUP BY film_id HAVING |",
			Require: []completiontest.Expected{{Text: "total_amount", Type: column}},
		},
		{
			Name:    "order by sees select alias",
			SQL:     "SELECT film_id, SUM(film_id) AS total_amount FROM film GROUP BY film_id ORDER BY |",
			Require: []completiontest.Expected{{Text: "total_amount", Type: column}},
		},
		{
			Name:    "where hides select alias",
			SQL:     "SELECT film_id, SUM(film_id) AS total_amount FROM film WHERE |",
			Exclude: []completiontest.Expected{{Text: "total_amount", Type: column}},
		},
		{
			Name:    "nested query sees only its select alias",
			SQL:     "SELECT film_id AS outer_alias FROM film WHERE EXISTS (SELECT actor_id AS inner_alias FROM film_actor GROUP BY |)",
			Require: []completiontest.Expected{{Text: "inner_alias", Type: column}},
			Exclude: []completiontest.Expected{{Text: "outer_alias", Type: column}},
		},
		{
			Name:    "outer query does not see nested select alias",
			SQL:     "SELECT film_id AS outer_alias, (SELECT actor_id AS inner_alias FROM film_actor LIMIT 1) FROM film ORDER BY |",
			Require: []completiontest.Expected{{Text: "outer_alias", Type: column}},
			Exclude: []completiontest.Expected{{Text: "inner_alias", Type: column}},
		},
		{
			Name: "insert target columns",
			SQL:  "INSERT INTO film (|)",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name: "update target columns",
			SQL:  "UPDATE film f SET |",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name: "delete target alias",
			SQL:  "DELETE FROM film f WHERE f.|",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name: "update join qualified alias",
			SQL:  "UPDATE film f JOIN film_actor fa ON f.film_id = fa.film_id SET fa.|",
			Require: []completiontest.Expected{
				{Text: "actor_id", Type: column}, {Text: "film_id", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "title", Type: column}},
		},
		{
			Name: "qualified partial prefix",
			SQL:  "SELECT * FROM store s WHERE s.store_|",
			Require: []completiontest.Expected{
				{Text: "store_name", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "id", Type: column}},
		},
		{
			Name:    "insert values are not column context",
			SQL:     "INSERT INTO film (film_id, title) VALUES (|)",
			Exclude: []completiontest.Expected{{Text: "film_id", Type: column}, {Text: "title", Type: column}},
		},
		{
			Name: "insert set uses target columns",
			SQL:  "INSERT INTO film SET |",
			Require: []completiontest.Expected{
				{Text: "film_id", Type: column}, {Text: "title", Type: column},
			},
		},
		{
			Name: "insert select uses source scope",
			SQL:  "INSERT INTO film (film_id, title) SELECT customer_id, | FROM customer c",
			Require: []completiontest.Expected{
				{Text: "customer_id", Type: column}, {Text: "email", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "title", Type: column}},
		},
		{
			Name: "CTE columns in joined update",
			SQL:  "WITH picked AS (SELECT customer_id, email FROM customer) UPDATE film f JOIN picked p ON f.film_id = p.customer_id SET f.title = p.|",
			Require: []completiontest.Expected{
				{Text: "customer_id", Type: column}, {Text: "email", Type: column},
			},
			Exclude: []completiontest.Expected{{Text: "title", Type: column}},
		},
	})
}

func TestCompleteClassifiesCursorContext(t *testing.T) {
	catalog := completiontest.Metadata("mysql", "sakila", "sakila")
	cases := []struct {
		name   string
		sql    string
		cursor int
		want   string
	}{
		{"column position", "SELECT  FROM orders", len("SELECT "), completioncore.PositionColumn},
		{"value position", "INSERT INTO orders VALUES (", len("INSERT INTO orders VALUES ("), completioncore.PositionValue},
		{"keyword position", "SELECT id FROM orders ", len("SELECT id FROM orders "), completioncore.PositionKeyword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx, err := coremysql.Complete(context.Background(), tc.sql, tc.cursor, nil, catalog)
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Position != tc.want {
				t.Fatalf("Position = %q, want %q", ctx.Position, tc.want)
			}
		})
	}
}

func TestCompletionWithBrokenTrailingStatementScalesLinearly(t *testing.T) {
	catalog := completiontest.Metadata("mysql", "sakila", "sakila")
	var sheet strings.Builder
	sheet.WriteString("SELECT s. FROM inventory i JOIN store s ON i.id = s.id;\n")
	sheet.WriteString("SELEC broken FROM oops;\n")
	for i := range 800 {
		fmt.Fprintf(&sheet, "SELECT col_a, col_b FROM table_%04d WHERE col_a = %d;\n", i, i)
	}
	started := time.Now()
	candidates, _, err := coremysql.Complete(context.Background(), sheet.String(), len("SELECT s."), nil, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) >= 2*time.Second {
		t.Fatalf("completion exceeded two-second regression bound")
	}
	found := false
	for _, candidate := range candidates {
		if candidate.Type == completioncore.CandidateColumn && candidate.Text == "store_name" {
			found = true
		}
	}
	if !found {
		t.Fatal("completion lost the caret statement while processing trailing SQL")
	}
}
