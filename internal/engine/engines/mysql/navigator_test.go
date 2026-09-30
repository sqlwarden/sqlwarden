package mysql

import (
	"context"
	"strings"
	"testing"

	mysqlconfig "github.com/go-sql-driver/mysql"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/enginetest"
	"github.com/sqlwarden/internal/engine/metadata"
)

func navExec(t *testing.T, d *Driver, stmts ...string) {
	t.Helper()
	for _, stmt := range stmts {
		if _, err := d.DB().ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

func navDatabase(name string) metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: name})
}

func navList(t *testing.T, d *Driver, nodeKind, folderKind string, parents ...metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
	t.Helper()
	folder, ok := d.Tree().Folder(nodeKind, folderKind)
	if !ok {
		t.Fatalf("folder %q under %q not declared", folderKind, nodeKind)
	}
	q, err := d.Querier(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	out, err := folder.List(context.Background(), q, parents)
	if err != nil {
		t.Fatalf("%s/%s: %v", nodeKind, folderKind, err)
	}
	return out
}

func childNamed(children []metadata.Child, kind, name string) (metadata.Child, bool) {
	for _, c := range children {
		if c.Kind == kind && c.Name == name {
			return c, true
		}
	}
	return metadata.Child{}, false
}

func connectAs(t *testing.T, mutate func(*mysqlconfig.Config)) *Driver {
	t.Helper()
	cfg, err := mysqlconfig.ParseDSN(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	mutate(cfg)
	d := &Driver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: cfg.FormatDSN(), Driver: "mysql"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestNavigatorTreeValidates(t *testing.T) {
	tree := (&Driver{}).Tree()
	if err := tree.Validate(); err != nil {
		t.Fatal(err)
	}
	if tree.DatabaseKind() != "database" {
		t.Fatalf("DatabaseKind = %q, want database", tree.DatabaseKind())
	}
	procedures, ok := tree.Folder("database", "procedures")
	if !ok || !procedures.Contains("function") || !procedures.Contains("procedure") {
		t.Fatalf("procedures folder must hold procedures and functions: %+v", procedures)
	}
}

func TestQuerierRequiresConnection(t *testing.T) {
	if _, err := (&Driver{}).Querier(context.Background(), "testdb"); err == nil {
		t.Fatal("Querier on an unconnected driver must fail")
	}
}

func TestNavigatorListsDatabases(t *testing.T) {
	d := newConnectedDriver(t)
	children := navList(t, d, "", "databases", "")[""]
	testdb, ok := childNamed(children, "database", "testdb")
	if !ok || !testdb.Current || testdb.System {
		t.Fatalf("testdb = %+v, %v; want current, not system", testdb, ok)
	}
	schema, ok := childNamed(children, "database", "information_schema")
	if !ok || !schema.System {
		t.Fatalf("information_schema = %+v, %v; want system", schema, ok)
	}
}

func TestNavigatorListsDatabasesWithoutDefault(t *testing.T) {
	d := connectAs(t, func(c *mysqlconfig.Config) { c.DBName = "" })
	children := navList(t, d, "", "databases", "")[""]
	if _, ok := childNamed(children, "database", "testdb"); !ok {
		t.Fatalf("testdb missing from %v", children)
	}
	for _, c := range children {
		if c.Current {
			t.Fatalf("%q must not be current without a default database", c.Name)
		}
	}
}

func TestNavigatorUsersDeniedIsEmpty(t *testing.T) {
	d := newConnectedDriver(t)
	if children := navList(t, d, "", "users", "")[""]; len(children) != 0 {
		t.Fatalf("users without mysql.user access = %v, want empty", children)
	}
}

func TestNavigatorListsUsers(t *testing.T) {
	d := connectAs(t, func(c *mysqlconfig.Config) { c.User = "root" })
	children := navList(t, d, "", "users", "")[""]
	user, ok := childNamed(children, "user", "testuser@%")
	if !ok || user.System {
		t.Fatalf("testuser@%% = %+v, %v; want listed, not system", user, ok)
	}
	sys, ok := childNamed(children, "user", "mysql.sys@localhost")
	if !ok || !sys.System {
		t.Fatalf("mysql.sys@localhost = %+v, %v; want system", sys, ok)
	}
}

func TestNavigatorListsDatabaseObjects(t *testing.T) {
	d := newConnectedDriver(t)
	navExec(t, d,
		`CREATE TABLE nav_users (id INT PRIMARY KEY, email VARCHAR(64), KEY nav_users_email (email))`,
		`CREATE VIEW nav_user_ids AS SELECT id FROM nav_users`,
		`CREATE PROCEDURE nav_touch() SELECT 1`,
		`CREATE FUNCTION nav_answer() RETURNS INT DETERMINISTIC NO SQL RETURN 42`,
		`CREATE TRIGGER nav_users_bi BEFORE INSERT ON nav_users FOR EACH ROW SET NEW.email = LOWER(NEW.email)`,
		`CREATE EVENT nav_tick ON SCHEDULE EVERY 1 DAY DISABLE DO SELECT 1`,
	)
	t.Cleanup(func() {
		navExec(t, d, `DROP EVENT IF EXISTS nav_tick`, `DROP FUNCTION IF EXISTS nav_answer`,
			`DROP PROCEDURE IF EXISTS nav_touch`, `DROP VIEW IF EXISTS nav_user_ids`, `DROP TABLE IF EXISTS nav_users`)
	})
	db := navDatabase("testdb")
	want := map[string][2]string{
		"tables":     {"table", "nav_users"},
		"views":      {"view", "nav_user_ids"},
		"indexes":    {"index", "nav_users_email"},
		"triggers":   {"trigger", "nav_users_bi"},
		"events":     {"event", "nav_tick"},
		"procedures": {"procedure", "nav_touch"},
	}
	for folder, kn := range want {
		if _, ok := childNamed(navList(t, d, "database", folder, db)[db], kn[0], kn[1]); !ok {
			t.Errorf("%s: missing %s %s", folder, kn[0], kn[1])
		}
	}
	if _, ok := childNamed(navList(t, d, "database", "procedures", db)[db], "function", "nav_answer"); !ok {
		t.Error("procedures: missing function nav_answer")
	}
	if _, ok := childNamed(navList(t, d, "database", "tables", db)[db], "table", "nav_user_ids"); ok {
		t.Error("tables must not list views")
	}
	idx, _ := childNamed(navList(t, d, "database", "indexes", db)[db], "index", "nav_users_email")
	if idx.Attributes["table"] != "nav_users" {
		t.Errorf("index table = %v, want nav_users", idx.Attributes["table"])
	}
	if _, ok := childNamed(navList(t, d, "database", "indexes", db)[db], "index", "PRIMARY"); ok {
		t.Error("database indexes must omit PRIMARY")
	}
}

func TestMySQLDatabaseNavigatorContract(t *testing.T) {
	d := newConnectedDriver(t)
	db := navDatabase("testdb")
	cases := []enginetest.NavigatorCase{
		{NodeKind: "", Folder: "databases", Parents: []metadata.ScopePath{""}},
	}
	for _, folder := range []string{"tables", "views", "indexes", "procedures", "triggers", "events"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "database", Folder: folder, Parents: []metadata.ScopePath{db}})
	}
	enginetest.RunNavigatorContract(t, d, "testdb", cases)
}

func navTable(name string) metadata.ScopePath {
	return navDatabase("testdb").Child(metadata.ScopeSegment{Kind: "table", Name: name})
}

func TestNavigatorListsTableChildren(t *testing.T) {
	d := newConnectedDriver(t)
	navExec(t, d,
		`CREATE TABLE nav_customers (id INT PRIMARY KEY, email VARCHAR(64) NOT NULL, CONSTRAINT nav_customers_email UNIQUE (email), CONSTRAINT nav_customers_id_positive CHECK (id > 0))`,
		`CREATE TABLE nav_orders (id INT NOT NULL, customer_id INT, KEY nav_orders_customer (customer_id), PRIMARY KEY (id), CONSTRAINT nav_orders_customer_fk FOREIGN KEY (customer_id) REFERENCES nav_customers (id))`,
		`CREATE TABLE nav_events (id INT NOT NULL, PRIMARY KEY (id)) PARTITION BY RANGE (id) (PARTITION p_low VALUES LESS THAN (100), PARTITION p_high VALUES LESS THAN MAXVALUE)`,
		`CREATE TRIGGER nav_orders_bi BEFORE INSERT ON nav_orders FOR EACH ROW SET NEW.id = NEW.id`,
		`CREATE VIEW nav_customer_emails AS SELECT email FROM nav_customers`,
	)
	t.Cleanup(func() {
		navExec(t, d, `DROP VIEW IF EXISTS nav_customer_emails`, `DROP TABLE IF EXISTS nav_orders`,
			`DROP TABLE IF EXISTS nav_customers`, `DROP TABLE IF EXISTS nav_events`)
	})
	customers, orders, events := navTable("nav_customers"), navTable("nav_orders"), navTable("nav_events")

	email, ok := childNamed(navList(t, d, "table", "columns", customers)[customers], "column", "email")
	if !ok || email.Attributes["data_type"] != "varchar(64)" || email.Attributes["nullable"] != false {
		t.Fatalf("email column = %+v", email)
	}
	id, _ := childNamed(navList(t, d, "table", "columns", customers)[customers], "column", "id")
	if id.Attributes["primary_key"] != true || id.Attributes["ordinal"] != int64(1) {
		t.Fatalf("id column = %+v", id)
	}
	customerID, _ := childNamed(navList(t, d, "table", "columns", orders)[orders], "column", "customer_id")
	if customerID.Attributes["foreign_key"] != true {
		t.Fatalf("customer_id column = %+v", customerID)
	}

	constraints := navList(t, d, "table", "constraints", customers)[customers]
	for name, typ := range map[string]string{"PRIMARY": "primary_key", "nav_customers_email": "unique", "nav_customers_id_positive": "check"} {
		c, ok := childNamed(constraints, "constraint", name)
		if !ok || c.Attributes["constraint_type"] != typ {
			t.Errorf("constraint %s = %+v, %v; want %s", name, c, ok, typ)
		}
	}

	fk, ok := childNamed(navList(t, d, "table", "foreign_keys", orders)[orders], "foreign_key", "nav_orders_customer_fk")
	if !ok || fk.Attributes["referenced_table"] != "testdb.nav_customers" {
		t.Fatalf("foreign key = %+v, %v", fk, ok)
	}
	ref, ok := childNamed(navList(t, d, "table", "references", customers)[customers], "reference", "nav_orders_customer_fk")
	if !ok || ref.Attributes["source_table"] != "testdb.nav_orders" {
		t.Fatalf("reference = %+v, %v", ref, ok)
	}

	trig, ok := childNamed(navList(t, d, "table", "triggers", orders)[orders], "trigger", "nav_orders_bi")
	if !ok || trig.Attributes["timing"] != "BEFORE" || trig.Attributes["event"] != "INSERT" {
		t.Fatalf("trigger = %+v, %v", trig, ok)
	}

	indexes := navList(t, d, "table", "indexes", orders)[orders]
	primary, ok := childNamed(indexes, "index", "PRIMARY")
	if !ok || primary.Attributes["primary"] != true || primary.Attributes["unique"] != true {
		t.Fatalf("PRIMARY index = %+v, %v", primary, ok)
	}
	secondary, ok := childNamed(indexes, "index", "nav_orders_customer")
	if !ok || secondary.Attributes["unique"] != false || secondary.Attributes["method"] != "btree" {
		t.Fatalf("secondary index = %+v, %v", secondary, ok)
	}

	partitions := navList(t, d, "table", "partitions", events)[events]
	if len(partitions) != 2 || partitions[0].Name != "p_low" || partitions[1].Name != "p_high" {
		t.Fatalf("partitions = %+v, want p_low, p_high in order", partitions)
	}
	if partitions[0].Attributes["method"] != "RANGE" || partitions[0].Attributes["bound"] != "100" {
		t.Fatalf("p_low = %+v", partitions[0])
	}

	view := navDatabase("testdb").Child(metadata.ScopeSegment{Kind: "view", Name: "nav_customer_emails"})
	if _, ok := childNamed(navList(t, d, "view", "columns", view)[view], "column", "email"); !ok {
		t.Fatal("view columns missing email")
	}
}

func TestNavigatorIndexNameSharedAcrossTables(t *testing.T) {
	d := newConnectedDriver(t)
	navExec(t, d,
		`CREATE TABLE nav_a (id INT PRIMARY KEY, created INT, KEY nav_created (created))`,
		`CREATE TABLE nav_b (id INT PRIMARY KEY, created INT, KEY nav_created (created))`,
	)
	t.Cleanup(func() { navExec(t, d, `DROP TABLE IF EXISTS nav_a`, `DROP TABLE IF EXISTS nav_b`) })
	db := navDatabase("testdb")
	var shared []metadata.Child
	for _, c := range navList(t, d, "database", "indexes", db)[db] {
		if c.Name == "nav_created" {
			shared = append(shared, c)
		}
	}
	if len(shared) != 1 || shared[0].Attributes["table"] != "nav_a, nav_b" {
		t.Fatalf("database indexes nav_created = %+v, want one entry for nav_a, nav_b", shared)
	}
	for _, table := range []string{"nav_a", "nav_b"} {
		if _, ok := childNamed(navList(t, d, "table", "indexes", navTable(table))[navTable(table)], "index", "nav_created"); !ok {
			t.Errorf("%s indexes missing nav_created", table)
		}
	}
	ref, _ := metadata.ObjectRefOf(navTable("nav_b").Child(metadata.ScopeSegment{Kind: "index", Name: "nav_created"}))
	desc, err := d.InspectDefinition(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if desc == nil || desc.Source == nil || !strings.Contains(desc.Source.Body, "`nav_b`") || strings.Contains(desc.Source.Body, "`nav_a`") {
		t.Fatalf("table-scoped index definition = %+v, want only nav_b", desc)
	}
}

func TestRelationLoadersAcceptEmptyBatch(t *testing.T) {
	d := newConnectedDriver(t)
	q, err := d.Querier(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"table", "view"} {
		node, _ := d.Tree().Node(kind)
		for _, f := range node.Folders {
			out, err := f.List(context.Background(), q, nil)
			if err != nil || len(out) != 0 {
				t.Errorf("%s/%s empty batch = %v, %v", kind, f.Kind, out, err)
			}
		}
	}
}

func TestMySQLRelationNavigatorContract(t *testing.T) {
	d := newConnectedDriver(t)
	navExec(t, d,
		`CREATE TABLE nav_parent (id INT PRIMARY KEY, name VARCHAR(10) UNIQUE)`,
		`CREATE TABLE nav_child (id INT PRIMARY KEY, parent_id INT, CONSTRAINT nav_child_parent FOREIGN KEY (parent_id) REFERENCES nav_parent (id))`,
		`CREATE TRIGGER nav_child_bi BEFORE INSERT ON nav_child FOR EACH ROW SET NEW.id = NEW.id`,
	)
	t.Cleanup(func() { navExec(t, d, `DROP TABLE IF EXISTS nav_child`, `DROP TABLE IF EXISTS nav_parent`) })
	tables := []metadata.ScopePath{navTable("nav_parent"), navTable("nav_child")}
	var cases []enginetest.NavigatorCase
	for _, folder := range []string{"columns", "constraints", "foreign_keys", "references", "triggers", "indexes", "partitions"} {
		cases = append(cases, enginetest.NavigatorCase{NodeKind: "table", Folder: folder, Parents: tables})
	}
	enginetest.RunNavigatorContract(t, d, "testdb", cases)
}
