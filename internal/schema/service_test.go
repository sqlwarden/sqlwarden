package schema

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeIntrospector struct {
	calls int32
	delay time.Duration
}

func (f *fakeIntrospector) Introspect(ctx context.Context, opts IntrospectOptions) (*Schema, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return &Schema{Connection: "c", Namespaces: []Namespace{{Name: "public"}}}, nil
}

func TestServiceGetCachesAfterMiss(t *testing.T) {
	svc := NewService(NewMemCache(8), time.Minute)
	intr := &fakeIntrospector{}

	if _, err := svc.Get(context.Background(), "c", intr); err != nil {
		t.Fatalf("first get: %v", err)
	}
	if _, err := svc.Get(context.Background(), "c", intr); err != nil {
		t.Fatalf("second get: %v", err)
	}
	if got := atomic.LoadInt32(&intr.calls); got != 1 {
		t.Fatalf("expected 1 introspection (second served from cache), got %d", got)
	}
}

func TestServiceGetSingleflightCollapsesConcurrentMisses(t *testing.T) {
	svc := NewService(NewMemCache(8), time.Minute)
	intr := &fakeIntrospector{delay: 50 * time.Millisecond}

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			_, _ = svc.Get(context.Background(), "c", intr)
		})
	}
	wg.Wait()

	if got := atomic.LoadInt32(&intr.calls); got != 1 {
		t.Fatalf("expected singleflight to collapse to 1 introspection, got %d", got)
	}
}

type erroringIntrospector struct {
	calls int32
}

func (e *erroringIntrospector) Introspect(ctx context.Context, opts IntrospectOptions) (*Schema, error) {
	atomic.AddInt32(&e.calls, 1)
	return nil, errors.New("introspection boom")
}

func TestServiceGetPropagatesErrorAndDoesNotCache(t *testing.T) {
	svc := NewService(NewMemCache(8), time.Minute)
	intr := &erroringIntrospector{}

	if _, err := svc.Get(context.Background(), "c", intr); err == nil {
		t.Fatal("expected error from first get")
	}
	// A failed introspection must not be cached, so a second call retries.
	if _, err := svc.Get(context.Background(), "c", intr); err == nil {
		t.Fatal("expected error from second get")
	}
	if got := atomic.LoadInt32(&intr.calls); got != 2 {
		t.Fatalf("expected 2 introspections (failures not cached), got %d", got)
	}
}

func TestServiceRefreshReIntrospects(t *testing.T) {
	svc := NewService(NewMemCache(8), time.Minute)
	intr := &fakeIntrospector{}

	if _, err := svc.Get(context.Background(), "c", intr); err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Refresh(context.Background(), "c", intr); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := atomic.LoadInt32(&intr.calls); got != 2 {
		t.Fatalf("expected refresh to force a second introspection, got %d", got)
	}
}
