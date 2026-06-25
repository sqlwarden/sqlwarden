package schema

import (
	"testing"
	"time"
)

func TestMemCacheSetGetInvalidate(t *testing.T) {
	c := NewMemCache(8)
	c.Set("a", []byte("x"), time.Minute)
	if v, ok := c.Get("a"); !ok || string(v) != "x" {
		t.Fatalf("want x,true got %q,%v", v, ok)
	}
	c.Invalidate("a")
	if _, ok := c.Get("a"); ok {
		t.Fatalf("expected miss after invalidate")
	}
}

func TestMemCacheExpires(t *testing.T) {
	c := NewMemCache(8)
	c.Set("a", []byte("x"), time.Nanosecond)
	time.Sleep(time.Millisecond)
	if _, ok := c.Get("a"); ok {
		t.Fatalf("expected expiry")
	}
}

func TestMemCacheLRUEviction(t *testing.T) {
	c := NewMemCache(2)
	c.Set("1", []byte("1"), time.Minute)
	c.Set("2", []byte("2"), time.Minute)
	_, _ = c.Get("1")                    // make "1" most-recently-used
	c.Set("3", []byte("3"), time.Minute) // should evict "2"
	if _, ok := c.Get("2"); ok {
		t.Fatal("expected '2' to be evicted")
	}
	if _, ok := c.Get("1"); !ok {
		t.Fatal("expected '1' to survive")
	}
}

func TestMemCacheInvalidatePrefix(t *testing.T) {
	c := NewMemCache(8)
	c.Set("obj:1\x00public\x00table\x00users", []byte("u"), time.Minute)
	c.Set("obj:1\x00public\x00table\x00orders", []byte("o"), time.Minute)
	c.Set("obj:12\x00public\x00table\x00x", []byte("x"), time.Minute) // different conn, must survive
	c.InvalidatePrefix("obj:1\x00")
	if _, ok := c.Get("obj:1\x00public\x00table\x00users"); ok {
		t.Errorf("expected prefix drop")
	}
	if _, ok := c.Get("obj:12\x00public\x00table\x00x"); !ok {
		t.Errorf("conn 12 must not be affected by conn 1 prefix")
	}
}
