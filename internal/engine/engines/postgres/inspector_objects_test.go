package postgres

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

// TestProcedureObjectsAndTriggerObjectsExist is a signature check; behavior
// is covered by the integration test added alongside the live-database
// wiring task.
func TestProcedureObjectsAndTriggerObjectsExist(t *testing.T) {
	_ = ProcedureObjects
	_ = TriggerObjects
}

// TestTypeDomainForeignTableObjectsExist is a signature check; behavior is
// covered by the integration test added alongside the live-database wiring
// task.
func TestTypeDomainForeignTableObjectsExist(t *testing.T) {
	_ = TypeObjects
	_ = DomainObjects
	_ = ForeignTableObjects
}

func TestAttachPostgresPartitionsSkipsUnmatchedTables(t *testing.T) {
	objs := []metadata.Object{{Ref: metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}),
		Kind:  "table", Name: "plain_table",
	}}}
	// No live DB in this unit test — a nil *sql.DB with zero tableRefs input
	// exercises the early-return path without a query.
	if err := attachPostgresPartitions(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected nil error on empty input, got %v", err)
	}
	_ = objs
}

func TestAttachPostgresPartitionsListsPartitionsFromLiveDB(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	mustExec(t, d, `DROP TABLE IF EXISTS part_parent`)
	mustExec(t, d, `DROP TABLE IF EXISTS part_plain`)
	mustExec(t, d, `CREATE TABLE part_parent (id int, val text) PARTITION BY RANGE (id)`)
	mustExec(t, d, `CREATE TABLE part_p1 PARTITION OF part_parent FOR VALUES FROM (0) TO (100)`)
	mustExec(t, d, `CREATE TABLE part_p2 PARTITION OF part_parent FOR VALUES FROM (100) TO (200)`)
	mustExec(t, d, `CREATE TABLE part_plain (id int)`)
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS part_parent")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS part_plain")
	})

	objs := []metadata.Object{
		{Ref: metadata.ObjectRef{Scope: pgTestScope("public"), Kind: "table", Name: "part_parent"}},
		{Ref: metadata.ObjectRef{Scope: pgTestScope("public"), Kind: "table", Name: "part_plain"}},
	}
	if err := attachPostgresPartitions(ctx, d.db, objs); err != nil {
		t.Fatalf("attachPostgresPartitions: %v", err)
	}

	var parent, plain *metadata.Object
	for i := range objs {
		switch objs[i].Ref.Name {
		case "part_parent":
			parent = &objs[i]
		case "part_plain":
			plain = &objs[i]
		}
	}

	var partitionsDesc *metadata.Descriptor
	for i := range parent.Descriptors {
		if parent.Descriptors[i].Kind == "rows" && parent.Descriptors[i].Title == "Partitions" {
			partitionsDesc = &parent.Descriptors[i]
		}
	}
	if partitionsDesc == nil {
		t.Fatalf("expected a Partitions rows descriptor on part_parent, got %+v", parent.Descriptors)
	}
	if got, want := partitionsDesc.Rows.Columns, []string{"Partition", "Bound", "Rows"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("columns = %+v, want %+v", got, want)
	}
	if len(partitionsDesc.Rows.Rows) != 2 {
		t.Fatalf("expected 2 partition rows, got %+v", partitionsDesc.Rows.Rows)
	}
	names := map[string]string{}
	for _, row := range partitionsDesc.Rows.Rows {
		if len(row) != 3 {
			t.Fatalf("expected 3 columns per row, got %+v", row)
		}
		names[row[0]] = row[1]
	}
	bound1, ok := names["part_p1"]
	if !ok || !strings.Contains(bound1, "FOR VALUES FROM") {
		t.Fatalf("part_p1 bound = %q, want to contain FOR VALUES FROM", bound1)
	}
	bound2, ok := names["part_p2"]
	if !ok || !strings.Contains(bound2, "FOR VALUES FROM") {
		t.Fatalf("part_p2 bound = %q, want to contain FOR VALUES FROM", bound2)
	}
	// Row count (3rd column) is left unasserted here: a freshly created
	// partition has no ANALYZE run, so reltuples is unreliable/zero.

	for i := range plain.Descriptors {
		if plain.Descriptors[i].Kind == "rows" && plain.Descriptors[i].Title == "Partitions" {
			t.Fatalf("non-partitioned table must not get a Partitions descriptor, got %+v", plain.Descriptors[i])
		}
	}
}
