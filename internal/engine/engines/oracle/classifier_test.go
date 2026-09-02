package oracle

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/classifier"
)

func TestOracleClassify(t *testing.T) {
	d := &oracleDriver{}
	cases := []struct {
		name string
		sql  string
		want classifier.Kind
	}{
		{"select", "SELECT * FROM employees", classifier.KindDQL},
		{"select from dual", "SELECT 1 FROM dual", classifier.KindDQL},
		{"select for update", "SELECT * FROM employees FOR UPDATE", classifier.KindUnknown},
		{"insert", "INSERT INTO t (a) VALUES (1)", classifier.KindDML},
		{"update", "UPDATE t SET a = 1", classifier.KindDML},
		{"delete", "DELETE FROM t", classifier.KindDML},
		{"merge", "MERGE INTO t USING s ON (t.id = s.id) WHEN MATCHED THEN UPDATE SET t.a = s.a", classifier.KindDML},
		{"create table", "CREATE TABLE t (a NUMBER)", classifier.KindDDL},
		{"alter table", "ALTER TABLE t ADD (b NUMBER)", classifier.KindDDL},
		{"drop", "DROP TABLE t", classifier.KindDDL},
		{"truncate", "TRUNCATE TABLE t", classifier.KindDDL},
		{"comment", "COMMENT ON TABLE t IS 'x'", classifier.KindDDL},
		{"grant", "GRANT SELECT ON t TO hr", classifier.KindDDL},
		{"anonymous block", "BEGIN NULL; END;", classifier.KindUnknown},
		{"syntax error", "SELECT FROM ( WHERE", classifier.KindUnknown},
		{"dql then ddl", "SELECT 1 FROM dual;\nDROP TABLE t", classifier.KindDDL},
		{"ddl then dml", "CREATE TABLE t (a NUMBER);\nINSERT INTO t VALUES (1)", classifier.KindUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.Classify(context.Background(), classifier.Request{SQL: tc.sql})
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if got.Kind != tc.want {
				t.Fatalf("Classify(%q).Kind = %v, want %v", tc.sql, got.Kind, tc.want)
			}
			if got.Source != "omni" {
				t.Errorf("Source = %q, want omni", got.Source)
			}
		})
	}
}
