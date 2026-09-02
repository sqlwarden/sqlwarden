//go:build integration

package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/cursor"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/metadata"
)

// testDSN is a go-ora URL for the APP_USER (schema WARDEN) of the shared
// container started in TestMain.
var testDSN string

// oracleITSchema is the APP_USER created by the gvenzl image; Oracle folds the
// unquoted name to upper case, so every scope in these tests uses "WARDEN".
const oracleITSchema = "WARDEN"

func TestMain(m *testing.M) {
	ctx := context.Background()

	const (
		image       = "gvenzl/oracle-free:23-slim-faststart"
		service     = "FREEPDB1"
		sysPassword = "warden_sys"
		appUser     = "warden"
		appPassword = "warden"
	)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			ExposedPorts: []string{"1521/tcp"},
			Env: map[string]string{
				"ORACLE_PASSWORD":   sysPassword,
				"APP_USER":          appUser,
				"APP_USER_PASSWORD": appPassword,
			},
			WaitingFor: wait.ForSQL("1521/tcp", "oracle", func(host string, port nat.Port) string {
				return fmt.Sprintf("oracle://%s:%s@%s:%s/%s", appUser, appPassword, host, port.Port(), service)
			}).WithQuery("SELECT 1 FROM DUAL").WithStartupTimeout(5 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start oracle container: %v\n", err)
		os.Exit(1)
	}

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "container host: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}
	mapped, err := container.MappedPort(ctx, "1521/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "container port: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = fmt.Sprintf("oracle://%s:%s@%s:%s/%s", appUser, appPassword, host, mapped.Port(), service)
	sysDSN := fmt.Sprintf("oracle://system:%s@%s:%s/%s", sysPassword, host, mapped.Port(), service)

	// RESOURCE (granted to APP_USER by the image) covers tables, sequences and
	// PL/SQL, but not views, materialized views, or the V$ fixed views that
	// DBMS_XPLAN.DISPLAY_CURSOR (EXPLAIN ANALYZE) reads. Grant those so every
	// schema object kind the inspector reports can be created from the tests
	// and the analyze-mode explain plan can be resolved.
	if err := grantPrivileges(ctx, sysDSN, appUser); err != nil {
		fmt.Fprintf(os.Stderr, "grant privileges: %v\n", err)
		_ = container.Terminate(ctx)
		os.Exit(1)
	}

	code := m.Run()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

func grantPrivileges(ctx context.Context, sysDSN, user string) error {
	db, err := sql.Open("oracle", sysDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, priv := range []string{"CREATE VIEW", "CREATE MATERIALIZED VIEW", "SELECT ANY DICTIONARY"} {
		if _, err := db.ExecContext(ctx, "GRANT "+priv+" TO "+user); err != nil {
			return fmt.Errorf("grant %q: %w", priv, err)
		}
	}
	return nil
}

func newConnectedDriver(t *testing.T) *oracleDriver {
	t.Helper()
	d := &oracleDriver{}
	if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "oracle"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func mustExec(t *testing.T, d *oracleDriver, query string) {
	t.Helper()
	if _, err := d.Execute(context.Background(), query); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// dropQuietly runs a DROP whose target may not exist; ORA-00942 (table or view
// does not exist) and ORA-02289 (sequence does not exist) are swallowed so
// cleanup ordering does not matter.
func dropQuietly(d *oracleDriver, statements ...string) {
	for _, s := range statements {
		_, _ = d.Execute(context.Background(), s)
	}
}

func itScope() metadata.ScopePath { return oracleSchemaScope(oracleITSchema) }

func currentHeapAlloc() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func heapGrowth(before, after uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

func attrString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func findColumn(cols []metadata.Column, name string) metadata.Column {
	for _, c := range cols {
		if c.Name == name {
			return c
		}
	}
	return metadata.Column{}
}

func descriptorByTitle(ds []metadata.Descriptor, title string) *metadata.Source {
	for _, d := range ds {
		if d.Title == title && d.Source != nil {
			return d.Source
		}
	}
	return nil
}

func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestOracleConnect(t *testing.T) {
	t.Run("valid DSN", func(t *testing.T) {
		d := &oracleDriver{}
		if err := d.Connect(context.Background(), engine.ConnectionConfig{DSN: testDSN, Driver: "oracle"}); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		_ = d.Close()
	})

	t.Run("invalid DSN", func(t *testing.T) {
		d := &oracleDriver{}
		err := d.Connect(context.Background(), engine.ConnectionConfig{
			DSN:    "oracle://warden:wrongpass@127.0.0.1:1/NOPE",
			Driver: "oracle",
		})
		if err == nil {
			_ = d.Close()
			t.Fatal("expected connect to fail with invalid DSN, got nil")
		}
	})

	t.Run("selected schema becomes current schema", func(t *testing.T) {
		d := &oracleDriver{}
		if err := d.Connect(context.Background(), engine.ConnectionConfig{
			DSN:          testDSN,
			Driver:       "oracle",
			DefaultScope: itScope(),
		}); err != nil {
			t.Fatalf("Connect: %v", err)
		}
		t.Cleanup(func() { _ = d.Close() })

		var current string
		if err := d.db.QueryRowContext(context.Background(),
			`SELECT SYS_CONTEXT('USERENV','CURRENT_SCHEMA') FROM DUAL`).Scan(&current); err != nil {
			t.Fatal(err)
		}
		if current != oracleITSchema {
			t.Fatalf("current schema = %q, want %q", current, oracleITSchema)
		}
	})
}

func TestOraclePing(t *testing.T) {
	d := newConnectedDriver(t)
	if err := d.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestOracleQueryBasicTypesAndArgs(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE basic_types_test") })

	mustExec(t, d, `CREATE TABLE basic_types_test (
		id       NUMBER(10) PRIMARY KEY,
		label    VARCHAR2(64),
		price    NUMBER(10,2),
		created  TIMESTAMP,
		body     CLOB,
		notes    VARCHAR2(64)
	)`)

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if _, err := d.Execute(ctx,
		`INSERT INTO basic_types_test (id, label, price, created, body, notes)
		 VALUES (:1, :2, :3, :4, :5, :6)`,
		1, "hello", 9.99, ts, "clob-body", nil); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rs, err := d.Query(ctx, `SELECT id, label, price, created, body, notes FROM basic_types_test WHERE id = :1`, 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rs.Columns) != 6 {
		t.Fatalf("columns = %d, want 6", len(rs.Columns))
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rs.Rows))
	}
	if got := rs.Rows[0][1].Text; got != "hello" {
		t.Errorf("label = %q, want hello", got)
	}
	if got := string(rs.Rows[0][5].Type); got != "null" {
		t.Errorf("notes type = %q, want null", got)
	}
}

func TestOracleExecuteDMLRowsAffected(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE dml_test") })

	mustExec(t, d, `CREATE TABLE dml_test (id NUMBER PRIMARY KEY, name VARCHAR2(32) NOT NULL)`)

	ins, err := d.Execute(ctx, `INSERT INTO dml_test
		SELECT 1, 'a' FROM DUAL UNION ALL SELECT 2, 'b' FROM DUAL UNION ALL SELECT 3, 'c' FROM DUAL`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if ins.RowsAffected == nil || *ins.RowsAffected != 3 {
		t.Fatalf("insert rows affected = %v, want 3", ins.RowsAffected)
	}

	upd, err := d.Execute(ctx, `UPDATE dml_test SET name = 'updated' WHERE id = 2`)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if upd.RowsAffected == nil || *upd.RowsAffected != 1 {
		t.Fatalf("update rows affected = %v, want 1", upd.RowsAffected)
	}

	del, err := d.Execute(ctx, `DELETE FROM dml_test WHERE id = 1`)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if del.RowsAffected == nil || *del.RowsAffected != 1 {
		t.Fatalf("delete rows affected = %v, want 1", del.RowsAffected)
	}
}

func TestOracleQueryCursorDoesNotMaterializeLargeResultSet(t *testing.T) {
	d := newConnectedDriver(t)

	const (
		totalRows        = 200_000
		payloadBytes     = 512
		pageSize         = 10
		maxHeapGrowthMiB = 64
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	before := currentHeapAlloc()
	qc, err := d.StartQuery(ctx, cursor.QueryRequest{SQL: fmt.Sprintf(`
		SELECT LEVEL AS n, RPAD('x', %d, 'x') AS payload
		FROM DUAL CONNECT BY LEVEL <= %d`, payloadBytes, totalRows)})
	if err != nil {
		t.Fatalf("StartQuery: %v", err)
	}
	defer qc.Close()

	rs, state, err := qc.Fetch(ctx, cursor.ScanOptions{MaxRows: pageSize})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if state.Exhausted {
		t.Fatal("expected cursor to remain open after first page")
	}
	if len(rs.Rows) != pageSize {
		t.Fatalf("rows = %d, want %d", len(rs.Rows), pageSize)
	}

	after := currentHeapAlloc()
	if growth := heapGrowth(before, after); growth > maxHeapGrowthMiB*1024*1024 {
		t.Fatalf("heap grew by %.2f MiB after fetching %d of %d rows; driver may be materializing the full result set",
			float64(growth)/(1024*1024), pageSize, totalRows)
	}
}

func TestOracleDialect(t *testing.T) {
	if got := (&oracleDriver{}).Dialect(); got != engine.DialectOracle {
		t.Fatalf("Dialect() = %q, want %q", got, engine.DialectOracle)
	}
}

func TestOracleInspectDirectory(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietly(d,
			"DROP MATERIALIZED VIEW dir_mv",
			"DROP VIEW dir_v",
			"DROP SEQUENCE dir_seq",
			"DROP TABLE dir_users",
			"DROP TABLE dir_orgs",
		)
	})
	mustExec(t, d, `CREATE TABLE dir_orgs (id NUMBER PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE dir_users (id NUMBER PRIMARY KEY, org_id NUMBER REFERENCES dir_orgs(id))`)
	mustExec(t, d, `INSERT INTO dir_users SELECT LEVEL, NULL FROM DUAL CONNECT BY LEVEL <= 5`)
	mustExec(t, d, `CREATE VIEW dir_v AS SELECT id FROM dir_users`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW dir_mv AS SELECT id FROM dir_users`)
	mustExec(t, d, `CREATE SEQUENCE dir_seq`)
	// InspectDirectory reads ALL_TABLES.NUM_ROWS, which the optimizer only
	// populates after stats are gathered.
	mustExec(t, d, `BEGIN DBMS_STATS.GATHER_TABLE_STATS(USER, 'DIR_USERS'); END;`)

	dir, err := d.InspectDirectory(ctx, metadata.DirectoryOptions{})
	if err != nil {
		t.Fatalf("InspectDirectory: %v", err)
	}
	if dir.Engine != "oracle" {
		t.Fatalf("engine = %q", dir.Engine)
	}

	var node *metadata.ScopeNode
	for _, n := range dir.ScopeNodes() {
		if n.Path.Name("schema") == oracleITSchema {
			copyNode := n
			node = &copyNode
		}
	}
	if node == nil {
		t.Fatalf("scope %q not in directory", oracleITSchema)
	}

	byKind := map[string][]string{}
	var tableGroup, viewGroup *metadata.ObjectGroup
	for i := range node.Groups {
		g := &node.Groups[i]
		for _, ref := range g.Objects {
			byKind[g.Kind] = append(byKind[g.Kind], ref.Name)
		}
		switch g.Kind {
		case "table":
			tableGroup = g
		case "view":
			viewGroup = g
		}
	}
	if !containsStr(byKind["table"], "DIR_USERS") || !containsStr(byKind["table"], "DIR_ORGS") {
		t.Errorf("tables = %v", byKind["table"])
	}
	if !containsStr(byKind["view"], "DIR_V") {
		t.Errorf("views = %v", byKind["view"])
	}
	if !containsStr(byKind["materialized_view"], "DIR_MV") {
		t.Errorf("materialized_views = %v", byKind["materialized_view"])
	}
	if !containsStr(byKind["sequence"], "DIR_SEQ") {
		t.Errorf("sequences = %v", byKind["sequence"])
	}
	if tableGroup == nil || tableGroup.RowCounts["DIR_USERS"] != 5 {
		t.Fatalf("DIR_USERS row count = %v, want 5", tableGroup)
	}
	if viewGroup != nil {
		if _, ok := viewGroup.RowCounts["DIR_V"]; ok {
			t.Fatalf("views must not report a row count: %+v", viewGroup.RowCounts)
		}
	}
}

func TestOracleDiscoverScopes(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE ds_probe") })
	mustExec(t, d, `CREATE TABLE ds_probe (id NUMBER)`)

	got, err := d.DiscoverScopes(ctx, metadata.ScopeDiscoveryRequest{})
	if err != nil {
		t.Fatalf("DiscoverScopes: %v", err)
	}
	var names []string
	for _, s := range got.Scopes {
		names = append(names, s.Name("schema"))
	}
	if !containsStr(names, oracleITSchema) {
		t.Fatalf("scopes %v missing %q", names, oracleITSchema)
	}
	for _, sys := range []string{"SYS", "SYSTEM", "XDB"} {
		if containsStr(names, sys) {
			t.Errorf("scopes leak Oracle-maintained schema %q: %v", sys, names)
		}
	}
	if got.Current.Name("schema") != oracleITSchema {
		t.Errorf("current scope = %q, want %q", got.Current.Name("schema"), oracleITSchema)
	}
}

func TestOracleInspectObjectsRelational(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietly(d,
			"DROP TABLE io_orders",
			"DROP TABLE io_customers",
		)
	})
	mustExec(t, d, `CREATE TABLE io_customers (id NUMBER PRIMARY KEY, email VARCHAR2(255))`)
	mustExec(t, d, `CREATE TABLE io_orders (
		id       NUMBER PRIMARY KEY,
		cust_id  NUMBER NOT NULL,
		total    NUMBER(12,2),
		CONSTRAINT io_orders_cust_fk FOREIGN KEY (cust_id) REFERENCES io_customers(id)
	)`)
	mustExec(t, d, `CREATE INDEX io_orders_cust_total_idx ON io_orders (cust_id, total)`)
	mustExec(t, d, `COMMENT ON TABLE io_orders IS 'customer orders'`)
	mustExec(t, d, `COMMENT ON COLUMN io_orders.total IS 'order total'`)

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{
		{Scope: itScope(), Kind: "table", Name: "IO_ORDERS"},
	})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatalf("expected one relational object, got %+v", objs)
	}
	o := objs[0]

	if len(o.Relational.PrimaryKey) != 1 || o.Relational.PrimaryKey[0] != "ID" {
		t.Errorf("primary key = %v", o.Relational.PrimaryKey)
	}
	if len(o.Relational.ForeignKeys) != 1 {
		t.Fatalf("foreign keys = %+v", o.Relational.ForeignKeys)
	}
	fk := o.Relational.ForeignKeys[0]
	if strings.Join(fk.Columns, ",") != "CUST_ID" || strings.Join(fk.ReferencedColumns, ",") != "ID" {
		t.Errorf("fk columns = %+v", fk)
	}
	if fk.References.Scope.Name("schema") != oracleITSchema || fk.References.Name != "IO_CUSTOMERS" {
		t.Errorf("fk reference not qualified: %+v", fk.References)
	}

	var idx *metadata.SecondaryIndex
	for i := range o.Relational.Indexes {
		if o.Relational.Indexes[i].Name == "IO_ORDERS_CUST_TOTAL_IDX" {
			idx = &o.Relational.Indexes[i]
		}
	}
	if idx == nil {
		t.Fatalf("index IO_ORDERS_CUST_TOTAL_IDX not found: %+v", o.Relational.Indexes)
	}
	if strings.Join(idx.Columns, ",") != "CUST_ID,TOTAL" {
		t.Errorf("index columns = %v, want CUST_ID,TOTAL", idx.Columns)
	}

	if got := attrString(o.Attributes, "comment"); got != "customer orders" {
		t.Errorf("table comment = %q", got)
	}
	if got := attrString(findColumn(o.Relational.Columns, "TOTAL").Attributes, "comment"); got != "order total" {
		t.Errorf("column comment = %q", got)
	}

	// DDL is no longer attached by bulk inspection; it is fetched lazily via
	// InspectDefinition (see TestOracleInspectDefinition).
	if descriptorByTitle(o.Descriptors, "DDL") != nil {
		t.Errorf("bulk InspectObjects should not attach DDL: %+v", o.Descriptors)
	}
}

func TestOracleInspectObjectsMaterializedViews(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietly(d,
			"DROP MATERIALIZED VIEW mv_a",
			"DROP MATERIALIZED VIEW mv_b",
			"DROP TABLE mv_base",
		)
	})
	mustExec(t, d, `CREATE TABLE mv_base (id NUMBER PRIMARY KEY, region VARCHAR2(40), amount NUMBER(12,2))`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW mv_a AS SELECT region, SUM(amount) total FROM mv_base GROUP BY region`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW mv_b AS SELECT id, amount FROM mv_base WHERE amount > 0`)

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{
		{Scope: itScope(), Kind: "materialized_view", Name: "MV_A"},
		{Scope: itScope(), Kind: "materialized_view", Name: "MV_B"},
	})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("expected two objects, got %+v", objs)
	}
	wantDef := map[string]string{"MV_A": "GROUP BY", "MV_B": "WHERE"}
	for _, o := range objs {
		if o.Relational == nil || len(o.Relational.Columns) == 0 {
			t.Errorf("%s: missing columns: %+v", o.Ref.Name, o)
		}
		def := descriptorByTitle(o.Descriptors, "Definition")
		if def == nil {
			t.Fatalf("%s: no Definition descriptor: %+v", o.Ref.Name, o.Descriptors)
		}
		if !strings.Contains(strings.ToUpper(def.Body), wantDef[o.Ref.Name]) {
			t.Errorf("%s: definition missing %q:\n%s", o.Ref.Name, wantDef[o.Ref.Name], def.Body)
		}
	}
}

// TestOracleInspectObjectsDictionaryTiersMatch pins the invariant behind the
// USER_* fast path: for objects in the connected schema, the cheap USER_* tier
// and the privilege-aware ALL_* tier must produce byte-identical metadata.
func TestOracleInspectObjectsDictionaryTiersMatch(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietly(d,
			"DROP MATERIALIZED VIEW dt_mv",
			"DROP TABLE dt_orders",
			"DROP TABLE dt_customers",
			"DROP SEQUENCE dt_seq",
		)
	})
	mustExec(t, d, `CREATE TABLE dt_customers (id NUMBER PRIMARY KEY, email VARCHAR2(255))`)
	mustExec(t, d, `CREATE TABLE dt_orders (
		id       NUMBER PRIMARY KEY,
		cust_id  NUMBER NOT NULL,
		total    NUMBER(12,2),
		CONSTRAINT dt_orders_cust_fk FOREIGN KEY (cust_id) REFERENCES dt_customers(id)
	)`)
	mustExec(t, d, `CREATE INDEX dt_orders_cust_total_idx ON dt_orders (cust_id, total)`)
	mustExec(t, d, `COMMENT ON TABLE dt_orders IS 'orders'`)
	mustExec(t, d, `COMMENT ON COLUMN dt_orders.total IS 'order total'`)
	mustExec(t, d, `CREATE MATERIALIZED VIEW dt_mv AS SELECT cust_id, SUM(total) total FROM dt_orders GROUP BY cust_id`)
	mustExec(t, d, `CREATE SEQUENCE dt_seq START WITH 5 INCREMENT BY 2`)

	relational := []metadata.ObjectRef{
		{Scope: itScope(), Kind: "table", Name: "DT_ORDERS"},
		{Scope: itScope(), Kind: "table", Name: "DT_CUSTOMERS"},
	}
	mviews := []metadata.ObjectRef{{Scope: itScope(), Kind: "materialized_view", Name: "DT_MV"}}
	sequences := []metadata.ObjectRef{{Scope: itScope(), Kind: "sequence", Name: "DT_SEQ"}}

	cases := []struct {
		name string
		run  func(dict oracleDict) ([]metadata.Object, error)
	}{
		{"relational", func(dict oracleDict) ([]metadata.Object, error) {
			return d.inspectRelational(ctx, dict, relational)
		}},
		{"materialized_views", func(dict oracleDict) ([]metadata.Object, error) {
			return d.inspectMaterializedViews(ctx, dict, mviews)
		}},
		{"sequences", func(dict oracleDict) ([]metadata.Object, error) {
			return d.inspectSequences(ctx, dict, sequences)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allTier, err := tc.run(oracleDict{})
			if err != nil {
				t.Fatalf("ALL_* tier: %v", err)
			}
			userTier, err := tc.run(oracleDict{user: true})
			if err != nil {
				t.Fatalf("USER_* tier: %v", err)
			}
			if !reflect.DeepEqual(allTier, userTier) {
				t.Fatalf("tier mismatch:\nALL_*  = %+v\nUSER_* = %+v", allTier, userTier)
			}
		})
	}
}

func TestOracleInspectDefinition(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietly(d, "DROP VIEW def_v", "DROP TABLE def_widgets")
	})
	mustExec(t, d, `CREATE TABLE def_widgets (id NUMBER PRIMARY KEY, label VARCHAR2(80))`)
	mustExec(t, d, `CREATE VIEW def_v AS SELECT id, label FROM def_widgets`)

	tbl, err := d.InspectDefinition(ctx, metadata.ObjectRef{Scope: itScope(), Kind: "table", Name: "DEF_WIDGETS"})
	if err != nil {
		t.Fatalf("InspectDefinition(table): %v", err)
	}
	if tbl == nil || tbl.Kind != "source" || tbl.Title != "DDL" {
		t.Fatalf("table definition descriptor = %+v", tbl)
	}
	for _, want := range []string{"CREATE TABLE", "DEF_WIDGETS", "LABEL"} {
		if !strings.Contains(strings.ToUpper(tbl.Source.Body), want) {
			t.Errorf("table DDL missing %q:\n%s", want, tbl.Source.Body)
		}
	}

	view, err := d.InspectDefinition(ctx, metadata.ObjectRef{Scope: itScope(), Kind: "view", Name: "DEF_V"})
	if err != nil {
		t.Fatalf("InspectDefinition(view): %v", err)
	}
	if view == nil || !strings.Contains(strings.ToUpper(view.Source.Body), "SELECT") {
		t.Fatalf("view definition descriptor = %+v", view)
	}

	seq, err := d.InspectDefinition(ctx, metadata.ObjectRef{Scope: itScope(), Kind: "sequence", Name: "WHATEVER"})
	if err != nil {
		t.Fatalf("InspectDefinition(sequence): %v", err)
	}
	if seq != nil {
		t.Errorf("unsupported kind should yield nil descriptor, got %+v", seq)
	}
}

func TestOracleInspectObjectsHonorsFilter(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE flt_a", "DROP TABLE flt_b") })
	mustExec(t, d, `CREATE TABLE flt_a (id NUMBER)`)
	mustExec(t, d, `CREATE TABLE flt_b (id NUMBER)`)

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{
		{Scope: itScope(), Kind: "table", Name: "FLT_A"},
	})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Ref.Name != "FLT_A" {
		t.Fatalf("filter not honored: %+v", objs)
	}
}

func TestOracleApplyDDL(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	scope := itScope()
	t.Cleanup(func() {
		dropQuietly(d, `DROP TABLE ad_widget CASCADE CONSTRAINTS`, `DROP TABLE ad_gadget CASCADE CONSTRAINTS`)
	})

	// create table
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     scope,
		Name:      "AD_WIDGET",
		Columns: []ddl.ColumnDefinition{
			{Name: "ID", DataType: "NUMBER", PrimaryKey: true},
			{Name: "OLD_NAME", DataType: "VARCHAR2(255)", Nullable: true},
			{Name: "SCRATCH", DataType: "NUMBER", Nullable: true},
		},
	}); err != nil {
		t.Fatalf("create table: %v", err)
	}
	assertColumns(t, d, "AD_WIDGET", []string{"ID", "OLD_NAME", "SCRATCH"})

	// rename column
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationRenameColumn,
		Ref:       &metadata.ObjectRef{Scope: scope, Kind: "table", Name: "AD_WIDGET"},
		Name:      "OLD_NAME",
		NewName:   "NEW_NAME",
	}); err != nil {
		t.Fatalf("rename column: %v", err)
	}
	assertColumns(t, d, "AD_WIDGET", []string{"ID", "NEW_NAME", "SCRATCH"})

	// drop column
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationDropColumn,
		Ref:       &metadata.ObjectRef{Scope: scope, Kind: "table", Name: "AD_WIDGET"},
		Name:      "SCRATCH",
	}); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	assertColumns(t, d, "AD_WIDGET", []string{"ID", "NEW_NAME"})

	// drop index
	mustExec(t, d, `CREATE INDEX ad_widget_name_idx ON ad_widget (new_name)`)
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationDropIndex,
		Ref:       &metadata.ObjectRef{Scope: scope, Kind: "table", Name: "AD_WIDGET"},
		Name:      "AD_WIDGET_NAME_IDX",
	}); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	var idxCount int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM all_indexes WHERE owner = :1 AND index_name = :2`,
		oracleITSchema, "AD_WIDGET_NAME_IDX").Scan(&idxCount); err != nil {
		t.Fatal(err)
	}
	if idxCount != 0 {
		t.Errorf("index still present after drop")
	}

	// drop object with CASCADE CONSTRAINTS
	mustExec(t, d, `CREATE TABLE ad_gadget (id NUMBER PRIMARY KEY,
		widget_id NUMBER REFERENCES ad_widget(id))`)
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationDropObject,
		Ref:       &metadata.ObjectRef{Scope: scope, Kind: "table", Name: "AD_WIDGET"},
		Cascade:   true,
	}); err != nil {
		t.Fatalf("drop object cascade: %v", err)
	}
	var tblCount int
	if err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM all_tables WHERE owner = :1 AND table_name = :2`,
		oracleITSchema, "AD_WIDGET").Scan(&tblCount); err != nil {
		t.Fatal(err)
	}
	if tblCount != 0 {
		t.Errorf("AD_WIDGET still present after drop")
	}
}

func assertColumns(t *testing.T, d *oracleDriver, table string, want []string) {
	t.Helper()
	rows, err := d.db.QueryContext(context.Background(),
		`SELECT column_name FROM all_tab_columns WHERE owner = :1 AND table_name = :2 ORDER BY column_id`,
		oracleITSchema, table)
	if err != nil {
		t.Fatalf("read columns: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s columns = %v, want %v", table, got, want)
	}
}

func TestOracleTransactions(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE tx_test", "DROP TABLE tx_ddl_marker") })
	mustExec(t, d, `CREATE TABLE tx_test (id NUMBER PRIMARY KEY)`)

	count := func() int {
		var n int
		if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tx_test`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	// commit persists
	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("begin: %v", err)
	}
	mustExec(t, d, `INSERT INTO tx_test (id) VALUES (1)`)
	if err := d.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if count() != 1 {
		t.Fatalf("after commit count = %d, want 1", count())
	}

	// rollback discards
	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("begin: %v", err)
	}
	mustExec(t, d, `INSERT INTO tx_test (id) VALUES (2)`)
	if err := d.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if count() != 1 {
		t.Fatalf("after rollback count = %d, want 1", count())
	}

	// savepoint / rollback-to-savepoint
	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("begin: %v", err)
	}
	mustExec(t, d, `INSERT INTO tx_test (id) VALUES (3)`)
	if err := d.Savepoint(ctx, "sp1"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	mustExec(t, d, `INSERT INTO tx_test (id) VALUES (4)`)
	if err := d.RollbackToSavepoint(ctx, "sp1"); err != nil {
		t.Fatalf("rollback to savepoint: %v", err)
	}
	if err := d.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if count() != 2 {
		t.Fatalf("after savepoint rollback count = %d, want 2 (ids 1,3)", count())
	}

	// DDL inside a transaction auto-commits pending DML
	if err := d.BeginTx(ctx); err != nil {
		t.Fatalf("begin: %v", err)
	}
	mustExec(t, d, `INSERT INTO tx_test (id) VALUES (5)`)
	mustExec(t, d, `CREATE TABLE tx_ddl_marker (id NUMBER)`)
	if err := d.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if count() != 3 {
		t.Fatalf("after DDL auto-commit count = %d, want 3 (id 5 committed by DDL)", count())
	}
}

func TestOracleInspectRelationshipsInScope(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietly(d, "DROP TABLE rel_line", "DROP TABLE rel_head")
	})
	mustExec(t, d, `CREATE TABLE rel_head (
		region VARCHAR2(8), num NUMBER,
		CONSTRAINT rel_head_pk PRIMARY KEY (region, num)
	)`)
	mustExec(t, d, `CREATE TABLE rel_line (
		id NUMBER PRIMARY KEY,
		head_region VARCHAR2(8),
		head_num NUMBER,
		CONSTRAINT rel_line_head_fk FOREIGN KEY (head_region, head_num)
			REFERENCES rel_head(region, num)
	)`)

	graph, err := d.InspectRelationshipsInScope(ctx, itScope())
	if err != nil {
		t.Fatalf("InspectRelationshipsInScope: %v", err)
	}
	var found *metadata.Relationship
	for i := range graph.Relationships {
		r := &graph.Relationships[i]
		if r.Source.Name == "REL_LINE" && r.References.Name == "REL_HEAD" {
			found = r
		}
	}
	if found == nil {
		t.Fatalf("edge REL_LINE->REL_HEAD not found: %+v", graph.Relationships)
	}
	if strings.Join(found.Columns, ",") != "HEAD_REGION,HEAD_NUM" {
		t.Errorf("fk columns = %v, want HEAD_REGION,HEAD_NUM (position order)", found.Columns)
	}
	if strings.Join(found.ReferencedColumns, ",") != "REGION,NUM" {
		t.Errorf("referenced columns = %v, want REGION,NUM", found.ReferencedColumns)
	}
	if found.References.Scope.Name("schema") != oracleITSchema || found.References.Kind != "table" {
		t.Errorf("edge reference not qualified: %+v", found.References)
	}
}

// TestOracleExplainPlanRoundTrips runs the two-statement Plan the Explainer
// produces against a live instance: EXPLAIN PLAN FOR ... then the DBMS_XPLAN
// query must return a readable plan that names the target table.
func TestOracleExplainPlanRoundTrips(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE explain_plan_test") })

	mustExec(t, d, `CREATE TABLE explain_plan_test (id NUMBER PRIMARY KEY, name VARCHAR2(32))`)

	plan, err := (&oracleDriver{}).Explain("SELECT id, name FROM explain_plan_test WHERE id = 1", explain.ModePlain)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	for _, stmt := range plan.Setup {
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}
	rs, err := d.Query(ctx, plan.Statement)
	if err != nil {
		t.Fatalf("plan query: %v", err)
	}
	if len(rs.Rows) == 0 {
		t.Fatal("expected plan output rows")
	}
	var b strings.Builder
	for _, row := range rs.Rows {
		b.WriteString(row[0].Text)
		b.WriteByte('\n')
	}
	out := b.String()
	if !strings.Contains(out, "EXPLAIN_PLAN_TEST") {
		t.Fatalf("plan output does not mention the table:\n%s", out)
	}
}

// TestOracleExplainAnalyzePlanRoundTrips exercises the ModeAnalyze Plan: it
// runs the statement for real with row-source statistics, reads the actual
// plan via DISPLAY_CURSOR, then restores STATISTICS_LEVEL via Teardown.
func TestOracleExplainAnalyzePlanRoundTrips(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE explain_analyze_test") })

	mustExec(t, d, `CREATE TABLE explain_analyze_test (id NUMBER PRIMARY KEY, name VARCHAR2(32))`)
	mustExec(t, d, `INSERT INTO explain_analyze_test
		SELECT LEVEL, 'row-' || LEVEL FROM DUAL CONNECT BY LEVEL <= 25`)

	plan, err := (&oracleDriver{}).Explain("SELECT COUNT(*) FROM explain_analyze_test", explain.ModeAnalyze)
	if err != nil {
		t.Fatalf("Explain: %v", err)
	}
	for _, stmt := range plan.Setup {
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}
	rs, err := d.Query(ctx, plan.Statement)
	if err != nil {
		t.Fatalf("plan query: %v", err)
	}
	for _, stmt := range plan.Teardown {
		if _, err := d.Execute(ctx, stmt); err != nil {
			t.Fatalf("teardown %q: %v", stmt, err)
		}
	}
	if len(rs.Rows) == 0 {
		t.Fatal("expected plan output rows")
	}
	var b strings.Builder
	for _, row := range rs.Rows {
		b.WriteString(row[0].Text)
		b.WriteByte('\n')
	}
	if out := b.String(); !strings.Contains(out, "EXPLAIN_ANALYZE_TEST") && !strings.Contains(strings.ToUpper(out), "SQL_ID") {
		t.Fatalf("plan output not a DISPLAY_CURSOR plan:\n%s", out)
	}

	rs, err = d.Query(ctx, "SELECT value FROM v$parameter WHERE name = 'statistics_level'")
	if err == nil && len(rs.Rows) == 1 && !strings.EqualFold(rs.Rows[0][0].Text, "TYPICAL") {
		t.Errorf("statistics_level = %q after teardown, want TYPICAL", rs.Rows[0][0].Text)
	}
}
