package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestCatalogEventsEnumeratesScheduledEvents(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	// Clean up any pre-existing events
	_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS test_event_1")
	_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS test_event_2")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS test_event_1")
		_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS test_event_2")
	})

	// Create test events
	if _, err := d.Execute(ctx, `CREATE EVENT test_event_1
		ON SCHEDULE EVERY 1 HOUR
		DO SELECT 1`); err != nil {
		t.Fatalf("create event 1: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE EVENT test_event_2
		ON SCHEDULE AT CURRENT_TIMESTAMP + INTERVAL 1 DAY
		DO SELECT 2`); err != nil {
		t.Fatalf("create event 2: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	type eventRef struct{ schema, name string }
	var events []eventRef
	if err := CatalogEvents(ctx, d.DB(), database, func(schema, name string) {
		events = append(events, eventRef{schema, name})
	}); err != nil {
		t.Fatalf("CatalogEvents: %v", err)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d: %+v", len(events), events)
	}

	found := map[string]bool{}
	for _, evt := range events {
		if evt.schema != database {
			t.Fatalf("unexpected schema %q for event %q, want %q", evt.schema, evt.name, database)
		}
		if evt.name == "test_event_1" || evt.name == "test_event_2" {
			found[evt.name] = true
		}
	}
	if !found["test_event_1"] {
		t.Fatalf("expected test_event_1 to be catalogued")
	}
	if !found["test_event_2"] {
		t.Fatalf("expected test_event_2 to be catalogued")
	}
}

func TestEventObjectsFetchesEventDetails(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	// Clean up
	_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS detail_event")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS detail_event")
	})

	// Create a test event with a specific schedule
	if _, err := d.Execute(ctx, `CREATE EVENT detail_event
		ON SCHEDULE EVERY 2 HOUR
		STARTS CURRENT_TIMESTAMP
		ENDS CURRENT_TIMESTAMP + INTERVAL 30 DAY
		DO SELECT 1`); err != nil {
		t.Fatalf("create event: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "event", Name: "detail_event"},
	}

	objects, err := EventObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("EventObjects: %v", err)
	}

	if len(objects) != 1 {
		t.Fatalf("expected 1 object, got %d: %+v", len(objects), objects)
	}

	obj := objects[0]
	if obj.Ref.Name != "detail_event" {
		t.Fatalf("expected name detail_event, got %q", obj.Ref.Name)
	}
	if obj.Ref.Kind != "event" {
		t.Fatalf("expected kind event, got %q", obj.Ref.Kind)
	}

	if len(obj.Descriptors) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(obj.Descriptors))
	}

	desc := obj.Descriptors[0]
	if desc.Kind != "fields" || desc.Title != "Event" {
		t.Fatalf("unexpected descriptor: kind=%q, title=%q", desc.Kind, desc.Title)
	}

	// Check fields
	if len(desc.Fields) < 2 {
		t.Fatalf("expected at least 2 fields (Type, Status), got %d: %+v", len(desc.Fields), desc.Fields)
	}

	hasType := false
	hasStatus := false
	hasInterval := false
	for _, field := range desc.Fields {
		if field.Name == "Type" {
			hasType = true
			if field.Value != "RECURRING" {
				t.Fatalf("expected Type=RECURRING, got %q", field.Value)
			}
		}
		if field.Name == "Status" {
			hasStatus = true
			// Status should be ENABLED or DISABLED
			if field.Value != "ENABLED" && field.Value != "DISABLED" {
				t.Fatalf("unexpected Status value: %q", field.Value)
			}
		}
		if field.Name == "Interval" {
			hasInterval = true
			// Should contain interval info for recurring event
		}
	}

	if !hasType {
		t.Fatalf("missing Type field: %+v", desc.Fields)
	}
	if !hasStatus {
		t.Fatalf("missing Status field: %+v", desc.Fields)
	}
	if !hasInterval {
		t.Fatalf("expected Interval field for recurring event, got %+v", desc.Fields)
	}
}

func TestEventObjectsHandlesNullIntervalField(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	// Clean up
	_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS one_time_event")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP EVENT IF EXISTS one_time_event")
	})

	// Create a one-time event (not recurring, so no interval)
	if _, err := d.Execute(ctx, `CREATE EVENT one_time_event
		ON SCHEDULE AT CURRENT_TIMESTAMP + INTERVAL 1 DAY
		DO SELECT 1`); err != nil {
		t.Fatalf("create one-time event: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "event", Name: "one_time_event"},
	}

	objects, err := EventObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("EventObjects: %v", err)
	}

	if len(objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objects))
	}

	obj := objects[0]
	desc := obj.Descriptors[0]

	// For ONE_TIME events, interval_value and interval_field should be NULL
	// So we should have only Type and Status fields, NOT an Interval field
	hasInterval := false
	for _, field := range desc.Fields {
		if field.Name == "Interval" {
			hasInterval = true
		}
	}
	if hasInterval {
		t.Fatalf("expected no Interval field for ONE_TIME event, got %+v", desc.Fields)
	}

	// Should still have Type and Status
	hasType := false
	hasStatus := false
	for _, field := range desc.Fields {
		if field.Name == "Type" {
			hasType = true
		}
		if field.Name == "Status" {
			hasStatus = true
		}
	}
	if !hasType || !hasStatus {
		t.Fatalf("expected Type and Status fields, got %+v", desc.Fields)
	}
}

func TestCatalogIndexesExcludesPrimary(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS idx_catalog_t1")
	t.Cleanup(func() { _, _ = d.Execute(ctx, "DROP TABLE IF EXISTS idx_catalog_t1") })

	if _, err := d.Execute(ctx, `CREATE TABLE idx_catalog_t1 (
		id INT PRIMARY KEY,
		email VARCHAR(255),
		INDEX idx_email (email)
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	type indexRef struct{ schema, name string }
	var indexes []indexRef
	if err := CatalogIndexes(ctx, d.DB(), database, func(schema, name string) {
		indexes = append(indexes, indexRef{schema, name})
	}); err != nil {
		t.Fatalf("CatalogIndexes: %v", err)
	}

	for _, idx := range indexes {
		if idx.name == "PRIMARY" {
			t.Fatalf("expected PRIMARY to be excluded from CatalogIndexes, got %+v", indexes)
		}
	}

	found := false
	for _, idx := range indexes {
		if idx.name == "idx_email" && idx.schema == database {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected idx_email to be catalogued, got %+v", indexes)
	}
}

func TestCatalogConstraintsEnumeratesTableConstraints(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_catalog_t1")
	t.Cleanup(func() { _, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_catalog_t1") })

	if _, err := d.Execute(ctx, `CREATE TABLE con_catalog_t1 (
		id INT PRIMARY KEY,
		email VARCHAR(255),
		CONSTRAINT uq_con_catalog_email UNIQUE (email)
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	type conRef struct{ schema, name string }
	var constraints []conRef
	if err := CatalogConstraints(ctx, d.DB(), database, func(schema, name string) {
		constraints = append(constraints, conRef{schema, name})
	}); err != nil {
		t.Fatalf("CatalogConstraints: %v", err)
	}

	found := false
	for _, con := range constraints {
		if con.name == "uq_con_catalog_email" && con.schema == database {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected uq_con_catalog_email to be catalogued, got %+v", constraints)
	}
}

// TestCatalogConstraintsDeduplicatesSharedNames proves CatalogConstraints
// emits one ref per (schema, constraint name) rather than one per underlying
// information_schema.table_constraints row. Every table has its own PRIMARY
// constraint row, so without the query's DISTINCT the tree would show one
// duplicate PRIMARY entry per table in the database.
func TestCatalogConstraintsDeduplicatesSharedNames(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	drop := func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_dedup_t1")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_dedup_t2")
	}
	drop()
	t.Cleanup(drop)

	for _, statement := range []string{
		`CREATE TABLE con_dedup_t1 (id INT PRIMARY KEY)`,
		`CREATE TABLE con_dedup_t2 (id INT PRIMARY KEY)`,
	} {
		if _, err := d.Execute(ctx, statement); err != nil {
			t.Fatalf("create table: %v", err)
		}
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	counts := map[string]int{}
	if err := CatalogConstraints(ctx, d.DB(), database, func(schema, name string) {
		counts[schema+"."+name]++
	}); err != nil {
		t.Fatalf("CatalogConstraints: %v", err)
	}
	if got := counts[database+".PRIMARY"]; got != 1 {
		t.Errorf("expected PRIMARY to be emitted exactly once, got %d", got)
	}
}

// TestIndexObjectsCollapsesSameNameAcrossTables proves the MySQL-specific
// limitation documented on CatalogIndexes: an index name shared by two
// different tables collapses to one (schema, name) tree entry, but
// IndexObjects still surfaces every table's instance as its own descriptor
// rather than overwriting/merging them.
func TestIndexObjectsCollapsesSameNameAcrossTables(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS idx_shared_a")
	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS idx_shared_b")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS idx_shared_a")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS idx_shared_b")
	})

	if _, err := d.Execute(ctx, `CREATE TABLE idx_shared_a (
		id INT PRIMARY KEY,
		created_at DATETIME,
		INDEX idx_created_at (created_at)
	)`); err != nil {
		t.Fatalf("create table a: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TABLE idx_shared_b (
		id INT PRIMARY KEY,
		name VARCHAR(50),
		created_at DATETIME,
		INDEX idx_created_at (name, created_at)
	)`); err != nil {
		t.Fatalf("create table b: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "index", Name: "idx_created_at"},
	}

	objects, err := IndexObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("IndexObjects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object (collapsed ref), got %d: %+v", len(objects), objects)
	}

	obj := objects[0]
	if len(obj.Descriptors) != 2 {
		t.Fatalf("expected 2 descriptors (one per table instance), got %d: %+v", len(obj.Descriptors), obj.Descriptors)
	}

	titles := map[string]metadata.Descriptor{}
	for _, d := range obj.Descriptors {
		titles[d.Title] = d
	}

	descA, ok := titles["Index on idx_shared_a"]
	if !ok {
		t.Fatalf("expected descriptor for idx_shared_a, got %+v", obj.Descriptors)
	}
	descB, ok := titles["Index on idx_shared_b"]
	if !ok {
		t.Fatalf("expected descriptor for idx_shared_b, got %+v", obj.Descriptors)
	}

	fieldValue := func(desc metadata.Descriptor, name string) string {
		for _, f := range desc.Fields {
			if f.Name == name {
				return f.Value
			}
		}
		return ""
	}

	if fieldValue(descA, "Columns") != "created_at" {
		t.Fatalf("expected idx_shared_a columns to be created_at, got %q", fieldValue(descA, "Columns"))
	}
	if fieldValue(descB, "Columns") != "name, created_at" {
		t.Fatalf("expected idx_shared_b columns to be name, created_at, got %q", fieldValue(descB, "Columns"))
	}
}

// TestConstraintObjectsCollapsesSameNameAcrossTables mirrors
// TestIndexObjectsCollapsesSameNameAcrossTables for ConstraintObjects.
func TestConstraintObjectsCollapsesSameNameAcrossTables(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_shared_a")
	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_shared_b")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_shared_a")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_shared_b")
	})

	if _, err := d.Execute(ctx, `CREATE TABLE con_shared_a (
		id INT PRIMARY KEY,
		email VARCHAR(255),
		CONSTRAINT uq_shared UNIQUE (email)
	)`); err != nil {
		t.Fatalf("create table a: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TABLE con_shared_b (
		id INT PRIMARY KEY,
		code VARCHAR(50),
		CONSTRAINT uq_shared UNIQUE (code)
	)`); err != nil {
		t.Fatalf("create table b: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "constraint", Name: "uq_shared"},
	}

	objects, err := ConstraintObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("ConstraintObjects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object (collapsed ref), got %d: %+v", len(objects), objects)
	}

	obj := objects[0]
	if len(obj.Descriptors) != 2 {
		t.Fatalf("expected 2 descriptors (one per table instance), got %d: %+v", len(obj.Descriptors), obj.Descriptors)
	}

	tables := map[string]bool{}
	for _, desc := range obj.Descriptors {
		for _, f := range desc.Fields {
			if f.Name == "Table" {
				tables[f.Value] = true
			}
		}
	}
	if !tables["con_shared_a"] || !tables["con_shared_b"] {
		t.Fatalf("expected both con_shared_a and con_shared_b represented, got %+v", obj.Descriptors)
	}
}

// TestConstraintObjectsSurfacesForeignKeyTarget proves a FOREIGN KEY
// constraint's descriptor includes "Referenced table"/"Referenced column"
// fields naming the target of the FK, and that a non-FK constraint
// (PRIMARY KEY) on the same table renders without those fields.
func TestConstraintObjectsSurfacesForeignKeyTarget(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_fk_child")
	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_fk_parent")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_fk_child")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS con_fk_parent")
	})

	if _, err := d.Execute(ctx, `CREATE TABLE con_fk_parent (
		id INT PRIMARY KEY,
		code VARCHAR(50) UNIQUE
	)`); err != nil {
		t.Fatalf("create parent table: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TABLE con_fk_child (
		id INT PRIMARY KEY,
		parent_code VARCHAR(50),
		CONSTRAINT fk_con_child_parent FOREIGN KEY (parent_code) REFERENCES con_fk_parent (code)
	)`); err != nil {
		t.Fatalf("create child table: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "constraint", Name: "fk_con_child_parent"},
		{Scope: scope, Kind: "constraint", Name: "PRIMARY"},
	}

	objects, err := ConstraintObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("ConstraintObjects: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("expected 2 objects, got %d: %+v", len(objects), objects)
	}

	fieldsByName := func(fields []metadata.Field, name string) (string, bool) {
		for _, f := range fields {
			if f.Name == name {
				return f.Value, true
			}
		}
		return "", false
	}

	fkObj := objects[0]
	if len(fkObj.Descriptors) != 1 {
		t.Fatalf("expected 1 descriptor for FK constraint, got %d: %+v", len(fkObj.Descriptors), fkObj.Descriptors)
	}
	fkFields := fkObj.Descriptors[0].Fields
	if table, ok := fieldsByName(fkFields, "Referenced table"); !ok || table != "con_fk_parent" {
		t.Fatalf("expected Referenced table = con_fk_parent, got %q (present=%v): %+v", table, ok, fkFields)
	}
	if col, ok := fieldsByName(fkFields, "Referenced column"); !ok || col != "code" {
		t.Fatalf("expected Referenced column = code, got %q (present=%v): %+v", col, ok, fkFields)
	}

	pkObj := objects[1]
	var childPKDescriptor *metadata.Descriptor
	for i := range pkObj.Descriptors {
		desc := &pkObj.Descriptors[i]
		if table, ok := fieldsByName(desc.Fields, "Table"); ok && table == "con_fk_child" {
			childPKDescriptor = desc
			break
		}
	}
	if childPKDescriptor == nil {
		t.Fatalf("expected a PRIMARY descriptor for con_fk_child, got %+v", pkObj.Descriptors)
	}
	if _, ok := fieldsByName(childPKDescriptor.Fields, "Referenced table"); ok {
		t.Fatalf("expected no Referenced table field on PRIMARY KEY constraint, got %+v", childPKDescriptor.Fields)
	}
	if _, ok := fieldsByName(childPKDescriptor.Fields, "Referenced column"); ok {
		t.Fatalf("expected no Referenced column field on PRIMARY KEY constraint, got %+v", childPKDescriptor.Fields)
	}
}

// TestIndexObjectsHandlesUnmatchedRef proves IndexObjects/ConstraintObjects
// don't panic when a ref has no matching rows (e.g. dropped concurrently).
func TestIndexObjectsHandlesUnmatchedRef(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})
	refs := []metadata.ObjectRef{
		{Scope: scope, Kind: "index", Name: "does_not_exist"},
	}

	objects, err := IndexObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("IndexObjects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected 1 object even with no matching rows, got %d", len(objects))
	}
	if len(objects[0].Descriptors) != 0 {
		t.Fatalf("expected no descriptors for unmatched ref, got %+v", objects[0].Descriptors)
	}

	conObjects, err := ConstraintObjects(ctx, d.DB(), refs)
	if err != nil {
		t.Fatalf("ConstraintObjects: %v", err)
	}
	if len(conObjects) != 1 {
		t.Fatalf("expected 1 object even with no matching rows, got %d", len(conObjects))
	}
	if len(conObjects[0].Descriptors) != 0 {
		t.Fatalf("expected no descriptors for unmatched ref, got %+v", conObjects[0].Descriptors)
	}
}

// TestIndexObjectsHandlesEmptyRefs proves IndexObjects/ConstraintObjects
// don't panic on an empty ref slice. An empty IN (...) list is a MySQL SQL
// syntax error (the same pre-existing limitation as every other refs-driven
// query in this package built on mysqlPairFilter), so callers are expected
// to guard on len(refs) > 0 as InspectObjects already does; this test only
// asserts the failure surfaces as a normal error, not a panic.
func TestIndexObjectsHandlesEmptyRefs(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	if _, err := IndexObjects(ctx, d.DB(), nil); err == nil {
		t.Fatalf("expected an error for empty refs, got nil")
	}

	if _, err := ConstraintObjects(ctx, d.DB(), nil); err == nil {
		t.Fatalf("expected an error for empty refs, got nil")
	}
}

// TestAttachMySQLPartitionsListsPartitionsFromLiveDB proves attachMySQLPartitions
// adds a "Partitions" rows descriptor (name, method, expression, row count) to a
// partitioned table object and sets its partition_method attribute, while a
// non-partitioned control table gets neither.
func TestAttachMySQLPartitionsListsPartitionsFromLiveDB(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS part_parent")
	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS part_plain")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS part_parent")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS part_plain")
	})

	if _, err := d.Execute(ctx, `CREATE TABLE part_parent (id int, val text)
		PARTITION BY RANGE (id) (
			PARTITION p0 VALUES LESS THAN (100),
			PARTITION p1 VALUES LESS THAN MAXVALUE
		)`); err != nil {
		t.Fatalf("create partitioned table: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TABLE part_plain (id int)`); err != nil {
		t.Fatalf("create plain table: %v", err)
	}

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: database})

	objs := []metadata.Object{
		{Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "part_parent"}},
		{Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "part_plain"}},
	}
	if err := attachMySQLPartitions(ctx, d.DB(), objs); err != nil {
		t.Fatalf("attachMySQLPartitions: %v", err)
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
	want := []string{"Partition", "Method", "Expression", "Rows"}
	if got := partitionsDesc.Rows.Columns; len(got) != len(want) {
		t.Fatalf("columns = %+v, want %+v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("columns = %+v, want %+v", got, want)
			}
		}
	}
	if len(partitionsDesc.Rows.Rows) != 2 {
		t.Fatalf("expected 2 partition rows, got %+v", partitionsDesc.Rows.Rows)
	}
	byName := map[string][]string{}
	for _, row := range partitionsDesc.Rows.Rows {
		if len(row) != 4 {
			t.Fatalf("expected 4 columns per row, got %+v", row)
		}
		byName[row[0]] = row
	}
	p0, ok := byName["p0"]
	if !ok {
		t.Fatalf("expected p0 partition, got %+v", byName)
	}
	if p0[1] != "RANGE" {
		t.Fatalf("p0 method = %q, want RANGE", p0[1])
	}
	if p0[2] != "`id`" {
		t.Fatalf("p0 expression = %q, want `id`", p0[2])
	}
	p1, ok := byName["p1"]
	if !ok {
		t.Fatalf("expected p1 partition, got %+v", byName)
	}
	if p1[1] != "RANGE" {
		t.Fatalf("p1 method = %q, want RANGE", p1[1])
	}

	if got, want := parent.Attributes["partition_method"], "RANGE"; got != want {
		t.Fatalf("partition_method attribute = %v, want %v", got, want)
	}

	if len(plain.Descriptors) != 0 {
		t.Fatalf("expected no descriptors on non-partitioned control table, got %+v", plain.Descriptors)
	}
	if plain.Attributes["partition_method"] != nil {
		t.Fatalf("expected no partition_method attribute on non-partitioned control table, got %v", plain.Attributes["partition_method"])
	}
}

// TestAttachMySQLPartitionsSkipsWhenNoTableRefs proves the early-return path
// when objs contains no table-kind refs (a nil *sql.DB would otherwise panic
// on query).
func TestAttachMySQLPartitionsSkipsWhenNoTableRefs(t *testing.T) {
	objs := []metadata.Object{{Ref: metadata.ObjectRef{
		Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "db"}),
		Kind:  "view", Name: "v1",
	}}}
	if err := attachMySQLPartitions(t.Context(), nil, objs); err != nil {
		t.Fatalf("expected nil error when no table refs present, got %v", err)
	}
}
