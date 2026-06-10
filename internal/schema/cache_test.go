package schema

import (
	"testing"
	"time"
)

func newSchema(conn string) *Schema {
	return &Schema{Connection: conn, Namespaces: []Namespace{{Name: "public"}}}
}

func TestMemCacheGetSetRoundTrip(t *testing.T) {
	c := NewMemCache(8)
	c.Set("1", newSchema("1"), time.Minute)
	got, ok := c.Get("1")
	if !ok {
		t.Fatal("expected hit")
	}
	if got.Connection != "1" || len(got.Namespaces) != 1 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestMemCacheTTLExpiry(t *testing.T) {
	c := NewMemCache(8)
	c.Set("1", newSchema("1"), time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, ok := c.Get("1"); ok {
		t.Fatal("expected expiry miss")
	}
}

func TestMemCacheLRUEviction(t *testing.T) {
	c := NewMemCache(2)
	c.Set("1", newSchema("1"), time.Minute)
	c.Set("2", newSchema("2"), time.Minute)
	_, _ = c.Get("1")                       // make "1" most-recently-used
	c.Set("3", newSchema("3"), time.Minute) // should evict "2"
	if _, ok := c.Get("2"); ok {
		t.Fatal("expected '2' to be evicted")
	}
	if _, ok := c.Get("1"); !ok {
		t.Fatal("expected '1' to survive")
	}
}

func TestMemCacheInvalidate(t *testing.T) {
	c := NewMemCache(8)
	c.Set("1", newSchema("1"), time.Minute)
	c.Invalidate("1")
	if _, ok := c.Get("1"); ok {
		t.Fatal("expected miss after invalidate")
	}
}
