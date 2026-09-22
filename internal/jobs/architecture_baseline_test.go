package jobs

import (
	"context"
	"testing"
	"time"
)

// BenchmarkArchitectureJobRoundTrip is the Phase 0 baseline for durable job
// enqueue, claim, and completion through the default database-backed queue.
func BenchmarkArchitectureJobRoundTrip(b *testing.B) {
	store, _ := newTestStore(b)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		record, err := store.Enqueue(ctx, EnqueueInput{Type: "baseline", Visibility: VisibilityInternal})
		if err != nil {
			b.Fatal(err)
		}
		claimed, found, err := store.ClaimDue(ctx, "baseline-worker", time.Now(), time.Minute)
		if err != nil {
			b.Fatal(err)
		}
		if !found || claimed.ID != record.ID {
			b.Fatalf("claimed %q, found=%t, want %q", claimed.ID, found, record.ID)
		}
		if err := store.Complete(ctx, claimed.ID, nil); err != nil {
			b.Fatal(err)
		}
	}
}
