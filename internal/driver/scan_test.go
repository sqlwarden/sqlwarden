package driver

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestScanRowsTruncatesByMaxRows(t *testing.T) {
	db := openScanTestDB(t)
	mustExecScanTest(t, db, "CREATE TABLE t (id INTEGER, name TEXT)")
	mustExecScanTest(t, db, "INSERT INTO t (id, name) VALUES (1, 'one'), (2, 'two'), (3, 'three')")

	rows, err := db.QueryContext(context.Background(), "SELECT id, name FROM t ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}

	rs, err := ScanRows(rows, ScanOptions{MaxRows: 2, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !rs.Truncated || rs.TruncationReason != TruncationReasonMaxRows {
		t.Fatalf("truncation = %v/%q, want max rows", rs.Truncated, rs.TruncationReason)
	}
	if rs.RowsReturned != 2 || len(rs.Rows) != 2 {
		t.Fatalf("rows returned = %d len=%d, want 2", rs.RowsReturned, len(rs.Rows))
	}
}

func TestScanRowsTruncatesByMaxBytesBeforeAddingOverflowRow(t *testing.T) {
	db := openScanTestDB(t)
	mustExecScanTest(t, db, "CREATE TABLE t (name TEXT)")
	mustExecScanTest(t, db, "INSERT INTO t (name) VALUES ('small'), ('"+strings.Repeat("x", 100)+"')")

	rows, err := db.QueryContext(context.Background(), "SELECT name FROM t ORDER BY rowid")
	if err != nil {
		t.Fatal(err)
	}

	rs, err := ScanRows(rows, ScanOptions{MaxRows: 10, MaxBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !rs.Truncated || rs.TruncationReason != TruncationReasonMaxBytes {
		t.Fatalf("truncation = %v/%q, want max bytes", rs.Truncated, rs.TruncationReason)
	}
	if rs.RowsReturned != 1 || len(rs.Rows) != 1 {
		t.Fatalf("rows returned = %d len=%d, want 1", rs.RowsReturned, len(rs.Rows))
	}
	if rs.BytesReturned != 5 {
		t.Fatalf("bytes returned = %d, want 5", rs.BytesReturned)
	}
}

func TestScanRowsFirstRowCanExceedMaxBytes(t *testing.T) {
	db := openScanTestDB(t)
	mustExecScanTest(t, db, "CREATE TABLE t (name TEXT)")
	mustExecScanTest(t, db, "INSERT INTO t (name) VALUES ('"+strings.Repeat("x", 100)+"')")

	rows, err := db.QueryContext(context.Background(), "SELECT name FROM t")
	if err != nil {
		t.Fatal(err)
	}

	rs, err := ScanRows(rows, ScanOptions{MaxRows: 10, MaxBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !rs.Truncated || rs.TruncationReason != TruncationReasonMaxBytes {
		t.Fatalf("truncation = %v/%q, want max bytes", rs.Truncated, rs.TruncationReason)
	}
	if len(rs.Columns) != 1 {
		t.Fatalf("columns = %d, want 1", len(rs.Columns))
	}
	if rs.RowsReturned != 0 || len(rs.Rows) != 0 || rs.BytesReturned != 0 {
		t.Fatalf("result = rows_returned=%d len=%d bytes=%d, want empty rows", rs.RowsReturned, len(rs.Rows), rs.BytesReturned)
	}
}

func openScanTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustExecScanTest(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query); err != nil {
		t.Fatal(err)
	}
}
