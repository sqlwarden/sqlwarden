package schema

import (
	"bytes"
	"compress/gzip"
	"container/list"
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Cache stores connection-keyed schema snapshots. Implementations must be
// safe for concurrent use. Values are keyed by connection ID (not by user)
// because schema is a property of the target database.
type Cache interface {
	Get(connID string) (*Schema, bool)
	Set(connID string, s *Schema, ttl time.Duration)
	Invalidate(connID string)
}

type cacheEntry struct {
	key       string
	data      []byte // gzip-compressed JSON
	expiresAt time.Time
}

// memCache is a bounded LRU cache with per-entry TTL. Snapshots are stored
// gzip-compressed so per-entry memory stays small and a future Redis-backed
// Cache can reuse the same byte representation.
type memCache struct {
	mu    sync.Mutex
	cap   int
	items map[string]*list.Element // key -> *list.Element holding *cacheEntry
	order *list.List               // front = most recently used
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

func (c *memCache) Get(connID string) (*Schema, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[connID]
	if !ok {
		return nil, false
	}
	ent := el.Value.(*cacheEntry)
	if time.Now().After(ent.expiresAt) {
		c.removeElement(el)
		return nil, false
	}
	c.order.MoveToFront(el)

	s, err := decodeSchema(ent.data)
	if err != nil {
		c.removeElement(el)
		return nil, false
	}
	return s, true
}

func (c *memCache) Set(connID string, s *Schema, ttl time.Duration) {
	data, err := encodeSchema(s)
	if err != nil {
		return // unservable snapshot; skip caching rather than poison the cache
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[connID]; ok {
		ent := el.Value.(*cacheEntry)
		ent.data = data
		ent.expiresAt = time.Now().Add(ttl)
		c.order.MoveToFront(el)
		return
	}

	ent := &cacheEntry{key: connID, data: data, expiresAt: time.Now().Add(ttl)}
	el := c.order.PushFront(ent)
	c.items[connID] = el

	for c.order.Len() > c.cap {
		c.removeElement(c.order.Back())
	}
}

func (c *memCache) Invalidate(connID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[connID]; ok {
		c.removeElement(el)
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

func encodeSchema(s *Schema) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if err := json.NewEncoder(gz).Encode(s); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeSchema(data []byte) (*Schema, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, err
	}
	var s Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
