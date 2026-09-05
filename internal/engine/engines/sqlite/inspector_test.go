package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestSQLiteInspectDefinition(t *testing.T) {
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()
	if _, err := d.Execute(context.Background(), `CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}),
		Kind:  "table", Name: "t",
	}
	desc, err := d.InspectDefinition(context.Background(), ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Source == nil || !strings.Contains(desc.Source.Body, "CREATE TABLE") {
		t.Fatalf("unexpected descriptor: %+v", desc)
	}
	if desc.Kind != "source" || desc.Source.Language != "sql" {
		t.Fatalf("descriptor shape: %+v", desc)
	}
}

func TestSQLiteInspectObjectsGeneratedAndCheck(t *testing.T) {
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()
	_, err := d.Execute(context.Background(), `
		CREATE TABLE t (
			id INTEGER PRIMARY KEY,
			price REAL NOT NULL CHECK (price >= 0),
			tax REAL GENERATED ALWAYS AS (price * 0.1) VIRTUAL,
			name TEXT COLLATE NOCASE
		) STRICT`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}),
		Kind:  "table", Name: "t",
	}
	objs, err := d.InspectObjects(context.Background(), []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatalf("no relational detail: %+v", objs)
	}
	cols := map[string]metadata.Column{}
	for _, c := range objs[0].Relational.Columns {
		cols[c.Name] = c
	}
	tax, ok := cols["tax"]
	if !ok {
		t.Fatalf("generated column 'tax' missing")
	}
	if got, _ := tax.Attributes["generated"].(string); got != "virtual" {
		t.Fatalf("tax generated attribute = %q, want virtual", got)
	}
	if got, _ := cols["name"].Attributes["collation"].(string); got != "NOCASE" {
		t.Fatalf("name collation attribute = %q, want NOCASE", got)
	}
	if got, _ := cols["price"].Attributes["check"].(string); !strings.Contains(got, ">= 0") {
		t.Fatalf("price check attribute = %q, want it to contain '>= 0'", got)
	}
	var constraints *metadata.Descriptor
	for i := range objs[0].Descriptors {
		if objs[0].Descriptors[i].Title == "Constraints" {
			constraints = &objs[0].Descriptors[i]
		}
	}
	if constraints == nil {
		t.Fatalf("missing Constraints descriptor: %+v", objs[0].Descriptors)
	}
	fields := map[string]string{}
	for _, f := range constraints.Fields {
		fields[f.Name] = f.Value
	}
	if !strings.Contains(fields["Options"], "STRICT") {
		t.Fatalf("Options field = %q, want it to contain STRICT", fields["Options"])
	}
	if !strings.Contains(fields["Checks"], ">= 0") {
		t.Fatalf("Checks field = %q, want it to contain '>= 0'", fields["Checks"])
	}
	if !strings.Contains(fields["Generated"], "tax") {
		t.Fatalf("Generated field = %q, want it to contain tax", fields["Generated"])
	}
}

func TestSQLiteInspectObjectsExpressionIndex(t *testing.T) {
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()
	if _, err := d.Execute(context.Background(), `CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := d.Execute(context.Background(), `CREATE INDEX t_lower_name ON t (lower(name), id)`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}),
		Kind:  "table", Name: "t",
	}
	objs, err := d.InspectObjects(context.Background(), []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatalf("no relational detail: %+v", objs)
	}
	var idx *metadata.SecondaryIndex
	for i := range objs[0].Relational.Indexes {
		if objs[0].Relational.Indexes[i].Name == "t_lower_name" {
			idx = &objs[0].Relational.Indexes[i]
		}
	}
	if idx == nil {
		t.Fatalf("index t_lower_name missing: %+v", objs[0].Relational.Indexes)
	}
	if len(idx.Columns) != 2 || idx.Columns[0] != "(expression)" || idx.Columns[1] != "id" {
		t.Fatalf("index columns = %+v, want [(expression) id]", idx.Columns)
	}
}

func TestSQLiteInspectObjectsWithoutRowidSecondaryIndex(t *testing.T) {
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()
	if _, err := d.Execute(context.Background(), `
		CREATE TABLE t (
			org_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			email TEXT NOT NULL,
			PRIMARY KEY (org_id, user_id)
		) WITHOUT ROWID`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := d.Execute(context.Background(), `CREATE INDEX t_email ON t (email)`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}),
		Kind:  "table", Name: "t",
	}
	objs, err := d.InspectObjects(context.Background(), []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatalf("no relational detail: %+v", objs)
	}
	var idx *metadata.SecondaryIndex
	for i := range objs[0].Relational.Indexes {
		if objs[0].Relational.Indexes[i].Name == "t_email" {
			idx = &objs[0].Relational.Indexes[i]
		}
	}
	if idx == nil {
		t.Fatalf("index t_email missing: %+v", objs[0].Relational.Indexes)
	}
	if len(idx.Columns) != 1 || idx.Columns[0] != "email" {
		t.Fatalf("index columns = %+v, want [email] (PK columns must not be appended)", idx.Columns)
	}
}

func TestSQLiteInspectDefinitionMissing(t *testing.T) {
	d := &sqliteDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()
	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"}),
		Kind:  "table", Name: "nope",
	}
	desc, err := d.InspectDefinition(context.Background(), ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc != nil {
		t.Fatalf("want nil descriptor for missing object, got %+v", desc)
	}
}
