// Package completiontest provides a reusable black-box scenario harness for
// dialect completion adapters.
package completiontest

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/dbengine/completioncore"
	"github.com/sqlwarden/internal/dbengine/metadata"
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

func Metadata(engine, database, namespace string) completioncore.MetadataResolver {
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	scope := root
	if engine == "postgres" {
		scope = root.Child(metadata.ScopeSegment{Kind: "schema", Name: namespace})
	}
	objects := []metadata.Object{
		relation(scope, "inventory",
			column("id", "bigint"), column("inventory_name", "text")),
		relation(scope, "store",
			column("id", "bigint"), column("store_name", "text")),
		relation(scope, "film",
			column("film_id", "smallint"), column("description", "text"), column("title", "text")),
		relation(scope, "film_actor",
			column("actor_id", "smallint"), column("film_id", "smallint")),
		relation(scope, "customer",
			column("customer_id", "bigint"), column("email", "text")),
	}
	refs := make([]metadata.ObjectRef, 0, len(objects))
	for _, object := range objects {
		refs = append(refs, object.Ref)
	}
	directory := &metadata.Directory{
		Engine: engine, DefaultScope: scope,
		Roots: []metadata.ScopeNode{{Path: scope, Groups: []metadata.ObjectGroup{{Kind: "table", Objects: refs}}}},
	}
	index := metadata.NewIndex(metadata.MetadataSet{Directory: directory, Objects: objects, Version: "test"})
	return completioncore.NewSchemaResolver(index, namespace)
}

func relation(scope metadata.ScopePath, name string, columns ...metadata.Column) metadata.Object {
	return metadata.Object{
		Ref:        metadata.ObjectRef{Scope: scope, Kind: "table", Name: name},
		Relational: &metadata.RelationalDetail{Columns: columns},
	}
}

func column(name, dataType string) metadata.Column {
	return metadata.Column{Name: name, DataType: dataType, Nullable: true}
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
