package connection

import (
	"runtime"
	"testing"
	"time"
)

// settledGoroutines waits for goroutine teardown to finish before counting, so
// a reaper that has been told to stop is not mistaken for a live one.
func settledGoroutines() int {
	count := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		time.Sleep(10 * time.Millisecond)
		next := runtime.NumGoroutine()
		if next >= count {
			return count
		}
		count = next
	}
	return count
}

// closeWithin fails the test if stop does not return before the deadline,
// which is how a manager that waits forever on a reaper that never ran shows
// up.
func closeWithin(t *testing.T, name string, stop func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		stop()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s.Close did not return", name)
	}
}

func TestManagerNewUnstartedRunsNoReaper(t *testing.T) {
	before := settledGoroutines()
	m := NewUnstarted(time.Minute)
	if after := settledGoroutines(); after != before {
		t.Fatalf("goroutines after NewUnstarted = %d, want %d", after, before)
	}

	m.StartReaper()
	if after := runtime.NumGoroutine(); after <= before {
		t.Fatalf("goroutines after StartReaper = %d, want more than %d", after, before)
	}

	m.Close()
	if after := settledGoroutines(); after != before {
		t.Fatalf("goroutines after Close = %d, want %d", after, before)
	}
}

func TestManagerCloseWithoutStartedReaper(t *testing.T) {
	m := NewUnstarted(time.Minute)
	closeWithin(t, "Manager", m.Close)
	closeWithin(t, "Manager", m.Close)

	before := settledGoroutines()
	m.StartReaper()
	if after := settledGoroutines(); after != before {
		t.Fatalf("StartReaper after Close started a reaper: goroutines %d, want %d", after, before)
	}
}

func TestQueryCursorManagerNewUnstartedRunsNoReaper(t *testing.T) {
	before := settledGoroutines()
	m := NewUnstartedQueryCursorManager(time.Minute)
	if after := settledGoroutines(); after != before {
		t.Fatalf("goroutines after NewUnstartedQueryCursorManager = %d, want %d", after, before)
	}

	m.StartReaper()
	if after := runtime.NumGoroutine(); after <= before {
		t.Fatalf("goroutines after StartReaper = %d, want more than %d", after, before)
	}

	m.Close()
	if after := settledGoroutines(); after != before {
		t.Fatalf("goroutines after Close = %d, want %d", after, before)
	}
}

func TestQueryCursorManagerCloseWithoutStartedReaper(t *testing.T) {
	m := NewUnstartedQueryCursorManager(time.Minute)
	closeWithin(t, "QueryCursorManager", m.Close)
	closeWithin(t, "QueryCursorManager", m.Close)

	before := settledGoroutines()
	m.StartReaper()
	if after := settledGoroutines(); after != before {
		t.Fatalf("StartReaper after Close started a reaper: goroutines %d, want %d", after, before)
	}
}
