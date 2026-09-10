package sqlserver

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	tcmssql "github.com/testcontainers/testcontainers-go/modules/mssql"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	mssqlContainer, err := tcmssql.Run(ctx,
		"mcr.microsoft.com/mssql/server:2022-latest",
		tcmssql.WithAcceptEULA(),
		tcmssql.WithPassword("Warden!Test123"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start sqlserver container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := mssqlContainer.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = mssqlContainer.Terminate(ctx)
		os.Exit(1)
	}
	testDSN = connStr

	code := m.Run()

	_ = mssqlContainer.Terminate(ctx)
	os.Exit(code)
}

func newConnectedDriver(t *testing.T) *Driver {
	t.Helper()
	d := &Driver{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := d.Connect(ctx, engine.ConnectionConfig{DSN: testDSN, Driver: "sqlserver"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestConnectQueryExecute(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	rs, err := d.Query(ctx, "SELECT 1 AS n")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rs.Rows))
	}

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_smoke (id INT NOT NULL)"); err != nil {
		t.Fatalf("Execute create: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_smoke") })

	execRs, err := d.Execute(ctx, "INSERT INTO dbo.warden_smoke (id) VALUES (1), (2)")
	if err != nil {
		t.Fatalf("Execute insert: %v", err)
	}
	if execRs.RowsAffected == nil || *execRs.RowsAffected != 2 {
		t.Fatalf("want 2 rows affected, got %+v", execRs.RowsAffected)
	}
}

func TestStartQueryCursor(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	c, err := d.StartQuery(ctx, cursor.QueryRequest{SQL: "SELECT 1 AS n UNION ALL SELECT 2"})
	if err != nil {
		t.Fatalf("StartQuery: %v", err)
	}
	defer c.Close()

	total := 0
	for {
		rs, state, err := c.Fetch(ctx, cursor.ScanOptions{})
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		total += len(rs.Rows)
		if state.Exhausted {
			break
		}
	}
	if total != 2 {
		t.Fatalf("want 2 rows, got %d", total)
	}
}

func TestTransactionSavepointRollback(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_tx (id INT NOT NULL)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_tx") })

	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	defer func() { _ = d.Rollback(ctx) }()

	if _, err := d.Execute(ctx, "INSERT INTO dbo.warden_tx (id) VALUES (1)"); err != nil {
		t.Fatalf("insert 1: %v", err)
	}
	if err := d.Savepoint(ctx, "sqlwarden_sp_1"); err != nil {
		t.Fatalf("Savepoint: %v", err)
	}
	if _, err := d.Execute(ctx, "INSERT INTO dbo.warden_tx (id) VALUES (2)"); err != nil {
		t.Fatalf("insert 2: %v", err)
	}
	if err := d.RollbackToSavepoint(ctx, "sqlwarden_sp_1"); err != nil {
		t.Fatalf("RollbackToSavepoint: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT COUNT(*) AS n FROM dbo.warden_tx")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rs.Rows) != 1 || rs.Rows[0][0].Integer != 1 {
		t.Fatalf("want count 1 (row 2 rolled back to savepoint), got rows=%v", rs.Rows)
	}
}

func TestCatalogTables(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_catalog_t (id INT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_catalog_t") })
	if _, err := d.Execute(ctx, "CREATE VIEW dbo.warden_catalog_v AS SELECT id FROM dbo.warden_catalog_t"); err != nil {
		t.Fatalf("create view: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP VIEW dbo.warden_catalog_v") })

	found := map[string]string{}
	err := CatalogTables(ctx, d.DB(), func(schema, name, kind string) {
		found[schema+"."+name] = kind
	})
	if err != nil {
		t.Fatalf("CatalogTables: %v", err)
	}
	if found["dbo.warden_catalog_t"] != "table" {
		t.Fatalf("want table entry, got %+v", found)
	}
	if found["dbo.warden_catalog_v"] != "view" {
		t.Fatalf("want view entry, got %+v", found)
	}
	for key := range found {
		schema := strings.SplitN(key, ".", 2)[0]
		if strings.EqualFold(schema, "INFORMATION_SCHEMA") {
			t.Fatalf("want no INFORMATION_SCHEMA entries, got %+v", found)
		}
	}
}

func TestRelationalObjectsColumnsAndIdentity(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, `CREATE TABLE dbo.warden_rel (
		id INT IDENTITY(1,1) PRIMARY KEY,
		name NVARCHAR(100) NOT NULL,
		parent_id INT NULL,
		CONSTRAINT fk_warden_rel_parent FOREIGN KEY (parent_id) REFERENCES dbo.warden_rel(id)
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_rel") })
	if _, err := d.Execute(ctx, "CREATE INDEX ix_warden_rel_name ON dbo.warden_rel(name)"); err != nil {
		t.Fatalf("create index: %v", err)
	}

	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"}),
		Kind:  "table",
		Name:  "warden_rel",
	}
	objs, err := RelationalObjects(ctx, d.DB(), []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("RelationalObjects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("want 1 object, got %d", len(objs))
	}
	obj := objs[0]
	if len(obj.Relational.Columns) != 3 {
		t.Fatalf("want 3 columns, got %d", len(obj.Relational.Columns))
	}
	if len(obj.Relational.PrimaryKey) != 1 || obj.Relational.PrimaryKey[0] != "id" {
		t.Fatalf("want PK [id], got %+v", obj.Relational.PrimaryKey)
	}
	if len(obj.Relational.ForeignKeys) != 1 {
		t.Fatalf("want 1 FK, got %+v", obj.Relational.ForeignKeys)
	}
	if len(obj.Relational.Indexes) != 1 {
		t.Fatalf("want 1 secondary index, got %+v", obj.Relational.Indexes)
	}
	idCol := obj.Relational.Columns[0]
	if idCol.Attributes["identity"] != "1,1" {
		t.Fatalf("want identity attribute on id column, got %+v", idCol.Attributes)
	}
}

func TestRelationalObjectsIndexExcludesIncludedColumns(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, `CREATE TABLE dbo.warden_rel_incl (
		id INT NOT NULL,
		a INT NOT NULL,
		b INT NOT NULL,
		c INT NOT NULL,
		e INT NOT NULL
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_rel_incl") })
	if _, err := d.Execute(ctx, "CREATE INDEX ix_warden_rel_incl ON dbo.warden_rel_incl(a, b) INCLUDE (c, e)"); err != nil {
		t.Fatalf("create index: %v", err)
	}

	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"}),
		Kind:  "table",
		Name:  "warden_rel_incl",
	}
	objs, err := RelationalObjects(ctx, d.DB(), []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("RelationalObjects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("want 1 object, got %d", len(objs))
	}
	obj := objs[0]
	if len(obj.Relational.Indexes) != 1 {
		t.Fatalf("want 1 secondary index, got %+v", obj.Relational.Indexes)
	}
	idx := obj.Relational.Indexes[0]
	if len(idx.Columns) != 2 || idx.Columns[0] != "a" || idx.Columns[1] != "b" {
		t.Fatalf("want key columns [a b], got %+v", idx.Columns)
	}
}

func TestRelationalObjectsEmptyRefsReturnsNilWithoutQuerying(t *testing.T) {
	ctx := context.Background()

	// nil db: any attempt to query would panic, proving the empty-refs guard
	// short-circuits before reaching the database.
	objs, err := RelationalObjects(ctx, nil, nil)
	if err != nil {
		t.Fatalf("RelationalObjects(nil refs): unexpected error: %v", err)
	}
	if objs != nil {
		t.Fatalf("want nil objects for empty refs, got %+v", objs)
	}

	objs, err = RelationalObjects(ctx, nil, []metadata.ObjectRef{})
	if err != nil {
		t.Fatalf("RelationalObjects(empty refs): unexpected error: %v", err)
	}
	if objs != nil {
		t.Fatalf("want nil objects for empty refs, got %+v", objs)
	}
}

func TestInspectDirectoryAndObjects(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_dir (id INT PRIMARY KEY)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_dir") })

	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	var tableRef *metadata.ObjectRef
	for _, ref := range dir.ObjectRefs() {
		if ref.Name == "warden_dir" {
			r := ref
			tableRef = &r
		}
	}
	if tableRef == nil {
		t.Fatalf("warden_dir not found in directory: %+v", dir)
	}

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{*tableRef})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || len(objs[0].Relational.Columns) != 1 {
		t.Fatalf("want 1 object with 1 column, got %+v", objs)
	}
}

func TestInspectDirectoryUsesOptsRootAsDefaultScope(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	var database string
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME()`).Scan(&database); err != nil {
		t.Fatalf("DB_NAME: %v", err)
	}

	dirNoOpts, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	wantDefault := metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: database},
	).Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	if d.DefaultScope() == "" && dirNoOpts.DefaultScope != wantDefault {
		t.Fatalf("want DefaultScope %q derived from the connected database/schema without opts.Root or a configured default scope, got %q", wantDefault, dirNoOpts.DefaultScope)
	}

	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "warden_root_override"})
	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{Root: root})
	if err != nil {
		t.Fatalf("InspectDirectory with Root: %v", err)
	}
	if dir.DefaultScope != root {
		t.Fatalf("want DefaultScope %q from opts.Root, got %q", root, dir.DefaultScope)
	}
}

func TestDiscoverScopesExcludesSystemSchemas(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE SCHEMA warden_scope_test"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP SCHEMA warden_scope_test") })

	var database string
	if err := d.db.QueryRowContext(ctx, `SELECT DB_NAME()`).Scan(&database); err != nil {
		t.Fatalf("DB_NAME: %v", err)
	}

	discovery, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{})
	if err != nil {
		t.Fatalf("DiscoverScopes: %v", err)
	}
	wantCurrent := metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: database},
	).Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	if discovery.Current != wantCurrent {
		t.Fatalf("want Current %q, got %q", wantCurrent, discovery.Current)
	}
	found := false
	for _, scope := range discovery.Scopes {
		if scope.Name("database") != database {
			t.Fatalf("want every scope nested under database %q, got %+v", database, discovery.Scopes)
		}
		if scope.Name("schema") == "warden_scope_test" {
			found = true
		}
		if scope.Name("schema") == "sys" || scope.Name("schema") == "INFORMATION_SCHEMA" {
			t.Fatalf("want no system schema in scopes, got %+v", discovery.Scopes)
		}
	}
	if !found {
		t.Fatalf("want warden_scope_test in scopes, got %+v", discovery.Scopes)
	}

	databaseRoot := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	nested, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{
		Parent: databaseRoot.Child(metadata.ScopeSegment{Kind: "schema", Name: "dbo"}),
	})
	if err != nil {
		t.Fatalf("DiscoverScopes with parent: %v", err)
	}
	if len(nested.Scopes) != 0 {
		t.Fatalf("want no scopes below a schema, got %+v", nested.Scopes)
	}

	otherDatabase, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{
		Parent: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database + "_other"}),
	})
	if err != nil {
		t.Fatalf("DiscoverScopes with other database parent: %v", err)
	}
	if len(otherDatabase.Scopes) != 0 {
		t.Fatalf("want no scopes for a different database, got %+v", otherDatabase.Scopes)
	}
}

func TestInspectDefinitionView(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_def_t (id INT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(context.Background(), "DROP VIEW dbo.warden_def_v; DROP TABLE dbo.warden_def_t")
	})
	if _, err := d.Execute(ctx, "CREATE VIEW dbo.warden_def_v AS SELECT id FROM dbo.warden_def_t"); err != nil {
		t.Fatalf("create view: %v", err)
	}

	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"}),
		Kind:  "view",
		Name:  "warden_def_v",
	}
	desc, err := d.InspectDefinition(ctx, ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Source == nil || desc.Source.Body == "" {
		t.Fatalf("want a non-empty definition, got %+v", desc)
	}
}

func TestInspectDefinitionTable(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, `
CREATE TABLE dbo.warden_def_t2 (
	id INT IDENTITY(1,1) NOT NULL,
	label NVARCHAR(50) NOT NULL DEFAULT ('x'),
	CONSTRAINT PK_warden_def_t2 PRIMARY KEY (id)
)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_def_t2") })

	ref := metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"}),
		Kind:  "table",
		Name:  "warden_def_t2",
	}
	desc, err := d.InspectDefinition(ctx, ref)
	if err != nil {
		t.Fatalf("InspectDefinition: %v", err)
	}
	if desc == nil || desc.Title != "DDL" || desc.Source == nil {
		t.Fatalf("want a DDL descriptor, got %+v", desc)
	}
	body := desc.Source.Body
	if !strings.Contains(body, "CREATE TABLE") ||
		!strings.Contains(body, "IDENTITY(1, 1)") ||
		!strings.Contains(body, "PRIMARY KEY ([id])") ||
		!strings.Contains(body, "NOT NULL") {
		t.Fatalf("table DDL missing expected clauses: %s", body)
	}

	missing, err := d.InspectDefinition(ctx, metadata.ObjectRef{Scope: ref.Scope, Kind: "table", Name: "warden_def_nope"})
	if err != nil {
		t.Fatalf("InspectDefinition(missing): %v", err)
	}
	if missing != nil {
		t.Errorf("missing table should yield a nil descriptor, got %+v", missing)
	}
}

func TestInspectModulesDirectoryObjectsAndDefinition(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_mod_t (id INT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(context.Background(), `
DROP TRIGGER dbo.warden_mod_trg;
DROP PROCEDURE dbo.warden_mod_proc;
DROP FUNCTION dbo.warden_mod_fn;
DROP TABLE dbo.warden_mod_t`)
	})
	if _, err := d.Execute(ctx, "CREATE PROCEDURE dbo.warden_mod_proc AS SELECT 1"); err != nil {
		t.Fatalf("create procedure: %v", err)
	}
	if _, err := d.Execute(ctx, "CREATE FUNCTION dbo.warden_mod_fn () RETURNS INT AS BEGIN RETURN 1 END"); err != nil {
		t.Fatalf("create function: %v", err)
	}
	if _, err := d.Execute(ctx, "CREATE TRIGGER dbo.warden_mod_trg ON dbo.warden_mod_t AFTER INSERT AS SELECT 1"); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	want := map[string]string{
		"warden_mod_proc": "procedure",
		"warden_mod_fn":   "function",
		"warden_mod_trg":  "trigger",
	}
	found := map[string]metadata.ObjectRef{}
	for _, ref := range dir.ObjectRefs() {
		if kind, ok := want[ref.Name]; ok && ref.Kind == kind {
			found[ref.Name] = ref
		}
	}
	if len(found) != len(want) {
		t.Fatalf("want %v in directory, found %+v", want, found)
	}

	refs := []metadata.ObjectRef{found["warden_mod_proc"], found["warden_mod_fn"], found["warden_mod_trg"]}
	objs, err := d.InspectObjects(ctx, refs)
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 3 {
		t.Fatalf("want 3 module objects, got %d: %+v", len(objs), objs)
	}
	for _, obj := range objs {
		if len(obj.Descriptors) == 0 || obj.Descriptors[0].Kind != "fields" {
			t.Fatalf("want a fields descriptor for %s, got %+v", obj.Ref.Name, obj.Descriptors)
		}
	}

	for _, ref := range refs {
		desc, err := d.InspectDefinition(ctx, ref)
		if err != nil {
			t.Fatalf("InspectDefinition(%s): %v", ref.Name, err)
		}
		if desc == nil || desc.Source == nil || desc.Source.Body == "" {
			t.Fatalf("want a non-empty definition for %s, got %+v", ref.Name, desc)
		}
	}
}

func TestInspectRelationshipsInScope(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	if _, err := d.Execute(ctx, "CREATE TABLE dbo.warden_rel_parent (id INT NOT NULL PRIMARY KEY)"); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_rel_child; DROP TABLE dbo.warden_rel_parent")
	})
	if _, err := d.Execute(ctx, `
CREATE TABLE dbo.warden_rel_child (
	id INT NOT NULL PRIMARY KEY,
	parent_id INT NOT NULL,
	CONSTRAINT FK_warden_rel_child_parent FOREIGN KEY (parent_id) REFERENCES dbo.warden_rel_parent (id)
)`); err != nil {
		t.Fatalf("create child: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	graph, err := d.InspectRelationshipsInScope(ctx, scope)
	if err != nil {
		t.Fatalf("InspectRelationshipsInScope: %v", err)
	}
	var found bool
	for _, rel := range graph.Relationships {
		if rel.Name != "FK_warden_rel_child_parent" {
			continue
		}
		found = true
		if rel.Source.Name != "warden_rel_child" || rel.References.Name != "warden_rel_parent" {
			t.Fatalf("unexpected relationship endpoints: %+v", rel)
		}
		if len(rel.Columns) != 1 || rel.Columns[0] != "parent_id" {
			t.Fatalf("unexpected FK columns: %+v", rel.Columns)
		}
		if len(rel.ReferencedColumns) != 1 || rel.ReferencedColumns[0] != "id" {
			t.Fatalf("unexpected referenced columns: %+v", rel.ReferencedColumns)
		}
	}
	if !found {
		t.Fatalf("expected FK_warden_rel_child_parent in %+v", graph.Relationships)
	}
}

func TestApplyDDLCreateAndDropTable(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	req := ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     scope,
		Name:      "warden_ddl_t",
		Columns: []ddl.ColumnDefinition{
			{Name: "id", DataType: "int", PrimaryKey: true},
			{Name: "label", DataType: "varchar(255)", Nullable: true},
		},
	}
	if err := d.ApplyDDL(ctx, req); err != nil {
		t.Fatalf("ApplyDDL create: %v", err)
	}

	dropReq := ddl.Request{
		Operation: ddl.OperationDropObject,
		Ref: &metadata.ObjectRef{
			Scope: scope,
			Kind:  "table",
			Name:  "warden_ddl_t",
		},
	}
	if err := d.ApplyDDL(ctx, dropReq); err != nil {
		t.Fatalf("ApplyDDL drop: %v", err)
	}
}

func TestApplyDDLRenameAndDropColumn(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "dbo"})
	createReq := ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     scope,
		Name:      "warden_ddl_rename_t",
		Columns: []ddl.ColumnDefinition{
			{Name: "id", DataType: "int", PrimaryKey: true},
			{Name: "old_label", DataType: "varchar(255)", Nullable: true},
			{Name: "extra", DataType: "varchar(255)", Nullable: true},
		},
	}
	if err := d.ApplyDDL(ctx, createReq); err != nil {
		t.Fatalf("ApplyDDL create: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(context.Background(), "DROP TABLE dbo.warden_ddl_rename_t")
	})

	tableRef := &metadata.ObjectRef{
		Scope: scope,
		Kind:  "table",
		Name:  "warden_ddl_rename_t",
	}

	renameReq := ddl.Request{
		Operation: ddl.OperationRenameColumn,
		Ref:       tableRef,
		Name:      "old_label",
		NewName:   "new_label",
	}
	if err := d.ApplyDDL(ctx, renameReq); err != nil {
		t.Fatalf("ApplyDDL rename column: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT new_label FROM dbo.warden_ddl_rename_t")
	if err != nil {
		t.Fatalf("query renamed column: %v", err)
	}
	if rs == nil {
		t.Fatalf("want a result set for renamed column")
	}

	dropColumnReq := ddl.Request{
		Operation: ddl.OperationDropColumn,
		Ref:       tableRef,
		Name:      "extra",
	}
	if err := d.ApplyDDL(ctx, dropColumnReq); err != nil {
		t.Fatalf("ApplyDDL drop column: %v", err)
	}

	if _, err := d.Query(ctx, "SELECT extra FROM dbo.warden_ddl_rename_t"); err == nil {
		t.Fatalf("want error querying dropped column, got none")
	}
}

func TestSqlServerSQLLiteralEscape(t *testing.T) {
	cases := map[string]string{
		"O'Brien":                  "O''Brien",
		"foo'; DROP TABLE bar; --": "foo''; DROP TABLE bar; --",
		"no quotes here":           "no quotes here",
		"''":                       "''''",
		"trailing'":                "trailing''",
	}
	for input, want := range cases {
		if got := sqlServerSQLLiteralEscape(input); got != want {
			t.Fatalf("sqlServerSQLLiteralEscape(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSqlServerDDLSQLRenameColumnEscapesQuotes(t *testing.T) {
	request := ddl.Request{
		Operation: ddl.OperationRenameColumn,
		Ref: &metadata.ObjectRef{
			Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "db'o"}),
			Kind:  "table",
			Name:  "tab'le",
		},
		Name:    "old'; DROP TABLE bar; --",
		NewName: "new'label",
	}

	statement, err := sqlServerDDLSQL(request)
	if err != nil {
		t.Fatalf("sqlServerDDLSQL: %v", err)
	}

	want := "EXEC sp_rename 'db''o.tab''le.old''; DROP TABLE bar; --', 'new''label', 'COLUMN'"
	if statement != want {
		t.Fatalf("sqlServerDDLSQL rename statement =\n%q\nwant\n%q", statement, want)
	}

	// Every raw single quote from the untrusted inputs must appear doubled in
	// the generated statement and nowhere as an unescaped quote that could
	// close a string literal early.
	if strings.Contains(statement, "db'o") || strings.Contains(statement, "tab'le") ||
		strings.Contains(statement, "old'; DROP") || strings.Contains(statement, "new'label") {
		t.Fatalf("sqlServerDDLSQL rename statement contains an unescaped quote: %q", statement)
	}
}
