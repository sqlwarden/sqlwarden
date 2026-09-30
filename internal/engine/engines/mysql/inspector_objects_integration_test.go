package mysql

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

// TestMySQLInspectObjectsCoversNewKinds proves every new schema-object kind
// added for Postgres/MySQL parity (event, index, constraint) is served by bulk
// object inspection against a live database, and that a partitioned table
// carries a Partitions descriptor.
func TestMySQLInspectObjectsCoversNewKinds(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyMySQL(t, d,
			"DROP TABLE IF EXISTS events_p",
			"DROP EVENT IF EXISTS test_event",
			"DROP TABLE IF EXISTS widgets",
		)
	})

	mustExecMySQL(t, d, `CREATE TABLE widgets (id INT PRIMARY KEY, label VARCHAR(100), created_at DATE)`)
	mustExecMySQL(t, d, `CREATE INDEX widgets_label_idx ON widgets (label)`)
	mustExecMySQL(t, d, `CREATE EVENT test_event ON SCHEDULE EVERY 1 DAY DO SELECT 1`)
	mustExecMySQL(t, d, `CREATE TABLE events_p (id INT, created_at DATE)
PARTITION BY RANGE (YEAR(created_at)) (
  PARTITION p2026 VALUES LESS THAN (2027)
)`)

	scope := mysqlTestScope()
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "table", Name: "widgets"},
		{Scope: scope, Kind: "index", Name: "widgets_label_idx"},
		{Scope: scope, Kind: "constraint", Name: "PRIMARY"},
		{Scope: scope, Kind: "event", Name: "test_event"},
		{Scope: scope, Kind: "table", Name: "events_p"},
	}

	objs, err := d.InspectObjects(ctx, refs)
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	inspected := map[string]bool{}
	for _, obj := range objs {
		inspected[obj.Ref.Kind+"/"+obj.Ref.Name] = true
	}
	for _, ref := range refs {
		if !inspected[ref.Kind+"/"+ref.Name] {
			t.Errorf("InspectObjects returned no object for %s %q", ref.Kind, ref.Name)
		}
	}
	var foundPartitions bool
	for _, obj := range objs {
		if obj.Ref.Name != "events_p" {
			continue
		}
		for _, desc := range obj.Descriptors {
			if desc.Title == "Partitions" && desc.Rows != nil &&
				len(desc.Rows.Rows) == 1 && desc.Rows.Rows[0][0] == "p2026" {
				foundPartitions = true
			}
		}
	}
	if !foundPartitions {
		t.Error("expected events_p to carry a Partitions descriptor listing p2026")
	}
}

// dropQuietlyMySQL runs cleanup DROP statements whose targets may already be
// gone, ignoring errors so cleanup ordering does not matter.
func dropQuietlyMySQL(t *testing.T, d *Driver, statements ...string) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range statements {
		_, _ = d.Execute(ctx, stmt)
	}
}

// mustExecMySQL runs a setup statement via the driver's own Execute path and
// fails the test immediately on error.
func mustExecMySQL(t *testing.T, d *Driver, sql string) {
	t.Helper()
	if _, err := d.Execute(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}
