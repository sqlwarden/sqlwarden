package schema

import (
	"context"
	"testing"
)

// BenchmarkArchitectureSchemaDirectoryCached is the Phase 0 baseline for the
// schema directory hot path used by API and completion requests.
func BenchmarkArchitectureSchemaDirectoryCached(b *testing.B) {
	service := newService()
	inspector := &fakeSchemaInspector{}
	ctx := context.Background()
	if _, err := service.Directory(ctx, "baseline", inspector); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := service.Directory(ctx, "baseline", inspector); err != nil {
			b.Fatal(err)
		}
	}
}
