package completer

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPreparedCacheCollapsesConcurrentBuildsAndEvictsLRU(t *testing.T) {
	cache := NewPreparedCache[int](2)
	var builds atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cache.GetOrBuild(context.Background(), "a", func() (int, error) {
				builds.Add(1)
				return 1, nil
			})
			if err != nil || got != 1 {
				t.Errorf("GetOrBuild() = %d, %v", got, err)
			}
		}()
	}
	wg.Wait()
	if got := builds.Load(); got != 1 {
		t.Fatalf("build count = %d, want 1", got)
	}

	_, _ = cache.GetOrBuild(context.Background(), "b", func() (int, error) { return 2, nil })
	_, _ = cache.GetOrBuild(context.Background(), "a", func() (int, error) { return 0, nil })
	_, _ = cache.GetOrBuild(context.Background(), "c", func() (int, error) { return 3, nil })
	if _, ok := cache.get("b"); ok {
		t.Fatal("least-recently-used entry was not evicted")
	}
}

func TestPreparedCacheInvalidationDuringBuildDoesNotReinsert(t *testing.T) {
	cache := NewPreparedCache[int](2)
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = cache.GetOrBuild(context.Background(), "connection:version", func() (int, error) {
			close(started)
			<-release
			return 1, nil
		})
	}()
	<-started
	cache.InvalidatePrefix("connection:")
	close(release)
	<-done
	if _, ok := cache.get("connection:version"); ok {
		t.Fatal("in-flight build was reinserted after invalidation")
	}
}
