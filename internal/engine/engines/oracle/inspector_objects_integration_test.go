//go:build integration

package oracle

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestOracleInspectSourceObjectKinds(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() {
		dropQuietly(d, "DROP PACKAGE source_pkg", "DROP TYPE source_type", "DROP TABLE source_table")
	})
	for _, sql := range []string{
		`CREATE TABLE source_table (id NUMBER)`,
		`CREATE OR REPLACE TRIGGER source_trigger BEFORE INSERT ON source_table FOR EACH ROW BEGIN :NEW.id := 42; END;`,
		`CREATE OR REPLACE PACKAGE source_pkg AS FUNCTION answer RETURN NUMBER; END;`,
		`CREATE OR REPLACE PACKAGE BODY source_pkg AS FUNCTION answer RETURN NUMBER IS BEGIN RETURN 42; END; END;`,
		`CREATE OR REPLACE TYPE source_type AS OBJECT (id NUMBER, MEMBER FUNCTION answer RETURN NUMBER)`,
		`CREATE OR REPLACE TYPE BODY source_type AS MEMBER FUNCTION answer RETURN NUMBER IS BEGIN RETURN self.id; END; END;`,
	} {
		mustExec(t, d, sql)
	}
	refs := []metadata.ObjectRef{
		{Scope: itScope(), Kind: "trigger", Name: "SOURCE_TRIGGER"},
		{Scope: itScope(), Kind: "package", Name: "SOURCE_PKG"},
		{Scope: itScope(), Kind: "package_body", Name: "SOURCE_PKG"},
		{Scope: itScope(), Kind: "type", Name: "SOURCE_TYPE"},
		{Scope: itScope(), Kind: "type_body", Name: "SOURCE_TYPE"},
	}
	directory, err := d.InspectDirectory(context.Background(), metadata.DirectoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[metadata.ObjectRef]bool{}
	for _, scope := range directory.ScopeNodes() {
		for _, group := range scope.Groups {
			for _, ref := range group.Objects {
				listed[ref] = true
			}
		}
	}
	for _, ref := range refs {
		if !listed[ref] {
			t.Errorf("directory is missing %+v", ref)
		}
	}
	for _, dict := range []oracleDict{{}, {user: true}} {
		objects, err := d.inspectRoutines(context.Background(), dict, refs)
		if err != nil {
			t.Fatal(err)
		}
		if len(objects) != len(refs) {
			t.Fatalf("got %d objects, want %d", len(objects), len(refs))
		}
		for _, object := range objects {
			source := descriptorByTitle(object.Descriptors, "Source")
			if source == nil {
				t.Errorf("missing source for %+v", object.Ref)
				continue
			}
			objectType := strings.ReplaceAll(strings.ToUpper(object.Ref.Kind), "_", " ")
			if !strings.HasPrefix(strings.ToUpper(source.Body), objectType+" ") {
				t.Errorf("wrong source for %s: %s", object.Ref.Kind, source.Body)
			}
			for _, descriptor := range object.Descriptors {
				for _, field := range descriptor.Fields {
					if field.Name == "Status" && field.Value != "VALID" {
						t.Errorf("%+v status = %s", object.Ref, field.Value)
					}
				}
			}
		}
	}
}

func TestOracleInspectCatalogObjectKinds(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() {
		dropQuietly(d, "DROP SYNONYM catalog_alias", "DROP DATABASE LINK catalog_link", "DROP TABLE catalog_table")
	})
	for _, sql := range []string{
		`CREATE TABLE catalog_table (id NUMBER CONSTRAINT catalog_pk PRIMARY KEY, amount NUMBER CONSTRAINT catalog_check CHECK (amount >= 0))`,
		`CREATE INDEX "catalog'index" ON catalog_table (amount)`,
		`CREATE SYNONYM catalog_alias FOR catalog_table`,
		`CREATE DATABASE LINK catalog_link CONNECT TO remote_user IDENTIFIED BY "test_only" USING 'unreachable.example/FREEPDB1'`,
	} {
		mustExec(t, d, sql)
	}
	directory, err := d.InspectDirectory(context.Background(), metadata.DirectoryOptions{})
	if err != nil {
		t.Fatal(err)
	}
	refs := []metadata.ObjectRef{
		{Scope: itScope(), Kind: "synonym", Name: "CATALOG_ALIAS"},
		{Scope: itScope(), Kind: "index", Name: "catalog'index"},
		{Scope: itScope(), Kind: "constraint", Name: "CATALOG_PK"},
		{Scope: itScope(), Kind: "constraint", Name: "CATALOG_CHECK"},
	}
	listed := map[metadata.ObjectRef]bool{}
	for _, scope := range directory.ScopeNodes() {
		for _, group := range scope.Groups {
			for _, ref := range group.Objects {
				listed[ref] = true
				// Oracle can append DB_DOMAIN to the database link name.
				if ref.Kind == "db_link" && strings.HasPrefix(ref.Name, "CATALOG_LINK") {
					refs = append(refs, ref)
				}
			}
		}
	}
	if len(refs) != 5 {
		t.Fatal("database link was not listed")
	}
	for _, ref := range refs {
		if !listed[ref] {
			t.Errorf("directory is missing %+v", ref)
		}
	}
	objects, err := d.InspectObjects(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != len(refs) {
		t.Fatalf("got %d objects, want %d", len(objects), len(refs))
	}
	for i, object := range objects {
		if object.Ref != refs[i] {
			t.Errorf("ref = %+v, want %+v", object.Ref, refs[i])
		}
		fields := map[string]string{}
		for _, descriptor := range object.Descriptors {
			for _, field := range descriptor.Fields {
				fields[field.Name] = field.Value
			}
		}
		var key, want string
		switch object.Ref.Kind {
		case "synonym":
			key, want = "Target object", "CATALOG_TABLE"
		case "index":
			key, want = "Table", "CATALOG_TABLE"
		case "constraint":
			key, want = "Type", "P"
			if object.Ref.Name == "CATALOG_CHECK" {
				key, want = "Check condition", "amount >= 0"
			}
		case "db_link":
			key, want = "Remote user", "REMOTE_USER"
		}
		if fields[key] != want {
			t.Errorf("%+v: %s = %q, want %q", object.Ref, key, fields[key], want)
		}
	}
	missing, err := d.InspectObjects(context.Background(), []metadata.ObjectRef{{Scope: itScope(), Kind: "synonym", Name: "MISSING_SYNONYM"}})
	if err != nil || len(missing) != 0 {
		t.Errorf("missing synonym = %+v, err = %v", missing, err)
	}
}

func TestOracleInspectTableStorage(t *testing.T) {
	d := newConnectedDriver(t)
	t.Cleanup(func() { dropQuietly(d, "DROP TABLE storage_table") })
	mustExec(t, d, `CREATE TABLE storage_table (id NUMBER GENERATED ALWAYS AS IDENTITY, amount NUMBER)
PARTITION BY RANGE (amount) (PARTITION p_small VALUES LESS THAN (100), PARTITION p_other VALUES LESS THAN (MAXVALUE))`)
	objects, err := d.InspectObjects(context.Background(), []metadata.ObjectRef{{Scope: itScope(), Kind: "table", Name: "STORAGE_TABLE"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 || objects[0].Relational == nil {
		t.Fatalf("objects = %+v", objects)
	}
	object := objects[0]
	if object.Attributes["partitioned"] != "YES" {
		t.Errorf("partitioned = %v", object.Attributes["partitioned"])
	}
	id := findColumn(object.Relational.Columns, "ID")
	if id.Attributes["identity"] != "ALWAYS" {
		t.Errorf("identity = %v", id.Attributes["identity"])
	}
	var partitions *metadata.RowSet
	for _, descriptor := range object.Descriptors {
		if descriptor.Title == "Partitions" {
			partitions = descriptor.Rows
		}
	}
	if partitions == nil || len(partitions.Rows) != 2 {
		t.Fatalf("partitions = %+v", partitions)
	}
	if partitions.Rows[0][0] != "P_SMALL" || partitions.Rows[1][0] != "P_OTHER" {
		t.Errorf("partitions = %+v", partitions.Rows)
	}
}
