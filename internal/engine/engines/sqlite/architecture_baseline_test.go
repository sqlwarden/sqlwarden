package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/sqlwarden/internal/engine"
)

// BenchmarkArchitectureQuery is the Phase 0 baseline for executing and fully
// materializing a small target-database query through an engine driver.
func BenchmarkArchitectureQuery(b *testing.B) {
	driver := &sqliteDriver{}
	if err := driver.Connect(context.Background(), engine.ConnectionConfig{
		DSN: filepath.Join(b.TempDir(), "query-baseline.db"), MaxResultRows: 1000, MaxResultBytes: 1 << 20,
	}); err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = driver.Close() })
	if _, err := driver.Execute(context.Background(), "CREATE TABLE baseline (id INTEGER PRIMARY KEY, value TEXT)"); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, err := driver.Execute(context.Background(), "INSERT INTO baseline(value) VALUES (?)", "value"); err != nil {
			b.Fatal(err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, err := driver.Query(context.Background(), "SELECT id, value FROM baseline ORDER BY id")
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Rows) != 100 {
			b.Fatalf("rows = %d, want 100", len(result.Rows))
		}
	}
}
