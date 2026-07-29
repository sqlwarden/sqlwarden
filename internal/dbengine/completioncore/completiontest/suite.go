// Package completiontest provides a reusable black-box scenario harness for
// dialect completion adapters.
package completiontest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/dbengine/completioncore"
	"github.com/sqlwarden/internal/dbengine/schema"
)

// CompleteFunc is the stable dialect-neutral shape exercised by the suite.
type CompleteFunc func(context.Context, string, int, completioncore.MetadataResolver) ([]completioncore.Candidate, error)

type Expected struct {
	Text string
	Type completioncore.CandidateType
}

// Scenario uses a single | marker as the caret. Require and Exclude are exact
// candidate assertions; this keeps fixtures resilient to new grammar keywords.
type Scenario struct {
	Name    string
	SQL     string
	Require []Expected
	Exclude []Expected
}

// Run executes scenarios as independently named subtests.
func Run(t *testing.T, complete CompleteFunc, catalog completioncore.MetadataResolver, scenarios []Scenario) {
	t.Helper()
	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			sql, cursor := Caret(t, scenario.SQL)
			candidates, err := complete(context.Background(), sql, cursor, catalog)
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range scenario.Require {
				if !contains(candidates, expected) {
					t.Fatalf("missing %s %q in %s", expected.Type, expected.Text, formatCandidates(candidates))
				}
			}
			for _, expected := range scenario.Exclude {
				if contains(candidates, expected) {
					t.Fatalf("unexpected %s %q in %s", expected.Type, expected.Text, formatCandidates(candidates))
				}
			}
		})
	}
}

func Caret(t *testing.T, marked string) (string, int) {
	t.Helper()
	if strings.Count(marked, "|") != 1 {
		t.Fatalf("scenario SQL must contain exactly one caret marker: %q", marked)
	}
	cursor := strings.IndexByte(marked, '|')
	return marked[:cursor] + marked[cursor+1:], cursor
}

func Catalog(dialect, database, namespace string) completioncore.MetadataResolver {
	objects := []schema.Object{
		relation(namespace, "inventory",
			column("id", "bigint"), column("inventory_name", "text")),
		relation(namespace, "store",
			column("id", "bigint"), column("store_name", "text")),
		relation(namespace, "film",
			column("film_id", "smallint"), column("description", "text"), column("title", "text")),
		relation(namespace, "film_actor",
			column("actor_id", "smallint"), column("film_id", "smallint")),
		relation(namespace, "customer",
			column("customer_id", "bigint"), column("email", "text")),
	}
	refs := make([]schema.ObjectRef, 0, len(objects))
	for _, object := range objects {
		refs = append(refs, object.Ref)
	}
	catalog := &schema.Catalog{
		Dialect: dialect, Database: database,
		Namespaces: []schema.NamespaceCatalog{{
			Name:   namespace,
			Groups: []schema.ObjectGroupCatalog{{Kind: "table", Objects: refs}},
		}},
	}
	index := schema.NewIndex(schema.MetadataSet{Catalog: catalog, Objects: objects, Version: "test"})
	return completioncore.NewSchemaResolver(index, namespace)
}

func relation(namespace, name string, columns ...schema.Column) schema.Object {
	return schema.Object{
		Ref:        schema.ObjectRef{Namespace: namespace, Kind: "table", Name: name},
		Relational: &schema.RelationalDetail{Columns: columns},
	}
}

func column(name, dataType string) schema.Column {
	return schema.Column{Name: name, DataType: dataType, Nullable: true}
}

func contains(candidates []completioncore.Candidate, expected Expected) bool {
	for _, candidate := range candidates {
		if candidate.Type == expected.Type && candidate.Text == expected.Text {
			return true
		}
	}
	return false
}

func formatCandidates(candidates []completioncore.Candidate) string {
	values := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		values = append(values, fmt.Sprintf("%s:%s", candidate.Type, candidate.Text))
	}
	return "[" + strings.Join(values, ", ") + "]"
}
