package schema

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

// Cache stores opaque byte values keyed by string. The schema Service derives
// keys from connection ID and ObjectRef, and stores gzip-compressed JSON, so a
// future Redis-backed Cache can reuse the same byte representation.
// Implementations must be safe for concurrent use.
type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, data []byte, ttl time.Duration)
	Invalidate(key string)
	InvalidatePrefix(prefix string)
}

type cacheEntry struct {
	key       string
	data      []byte
	expiresAt time.Time
}

// memCache is a bounded LRU cache with per-entry TTL.
type memCache struct {
	mu    sync.Mutex
	cap   int
	items map[string]*list.Element
	order *list.List // front = most recently used
}

// NewMemCache returns an in-memory Cache holding at most capacity entries.
func NewMemCache(capacity int) Cache {
	if capacity < 1 {
		capacity = 1
	}
	return &memCache{
		cap:   capacity,
		items: make(map[string]*list.Element, capacity),
		order: list.New(),
	}
}

func (c *memCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	ent := el.Value.(*cacheEntry)
	if time.Now().After(ent.expiresAt) {
		c.removeElement(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return ent.data, true
}

func (c *memCache) Set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		ent := el.Value.(*cacheEntry)
		ent.data = data
		ent.expiresAt = time.Now().Add(ttl)
		c.order.MoveToFront(el)
		return
	}

	ent := &cacheEntry{key: key, data: data, expiresAt: time.Now().Add(ttl)}
	el := c.order.PushFront(ent)
	c.items[key] = el

	for c.order.Len() > c.cap {
		c.removeElement(c.order.Back())
	}
}

func (c *memCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeElement(el)
	}
}

func (c *memCache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, el := range c.items {
		if strings.HasPrefix(key, prefix) {
			c.removeElement(el)
		}
	}
}

// removeElement must be called with c.mu held.
func (c *memCache) removeElement(el *list.Element) {
	if el == nil {
		return
	}
	c.order.Remove(el)
	delete(c.items, el.Value.(*cacheEntry).key)
}
