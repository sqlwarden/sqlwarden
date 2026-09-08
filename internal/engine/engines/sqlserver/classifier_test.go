package sqlserver

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/classifier"
)

func TestClassifySelect(t *testing.T) {
	d := &Driver{}
	res, err := d.Classify(context.Background(), classifier.Request{SQL: "SELECT * FROM dbo.t"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != classifier.KindDQL {
		t.Fatalf("want DQL, got %v", res.Kind)
	}
}

func TestClassifyInsertUpdateDelete(t *testing.T) {
	d := &Driver{}
	for _, sql := range []string{
		"INSERT INTO dbo.t (a) VALUES (1)",
		"UPDATE dbo.t SET a = 1",
		"DELETE FROM dbo.t",
		"MERGE INTO dbo.t USING dbo.s ON dbo.t.id = dbo.s.id WHEN MATCHED THEN UPDATE SET a = 1",
	} {
		res, err := d.Classify(context.Background(), classifier.Request{SQL: sql})
		if err != nil {
			t.Fatalf("%q: %v", sql, err)
		}
		if res.Kind != classifier.KindDML {
			t.Fatalf("%q: want DML, got %v", sql, res.Kind)
		}
	}
}

func TestClassifyDDL(t *testing.T) {
	d := &Driver{}
	for _, sql := range []string{
		"CREATE TABLE dbo.t (id INT)",
		"ALTER TABLE dbo.t ADD b INT",
		"DROP TABLE dbo.t",
		"TRUNCATE TABLE dbo.t",
	} {
		res, err := d.Classify(context.Background(), classifier.Request{SQL: sql})
		if err != nil {
			t.Fatalf("%q: %v", sql, err)
		}
		if res.Kind != classifier.KindDDL {
			t.Fatalf("%q: want DDL, got %v", sql, res.Kind)
		}
	}
}

func TestClassifyGoBatchIgnoredNotUnknownWhole(t *testing.T) {
	d := &Driver{}
	res, err := d.Classify(context.Background(), classifier.Request{SQL: "SELECT 1\nGO"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != classifier.KindDQL {
		t.Fatalf("want DQL (GO excluded), got %v", res.Kind)
	}
}

func TestClassifySyntaxErrorIsUnknown(t *testing.T) {
	d := &Driver{}
	res, err := d.Classify(context.Background(), classifier.Request{SQL: "SELEC 1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != classifier.KindUnknown {
		t.Fatalf("want Unknown, got %v", res.Kind)
	}
}
