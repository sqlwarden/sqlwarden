package completer

import (
	"container/list"
	"context"
	"strings"
	"sync"

	"golang.org/x/sync/singleflight"
)

type preparedEntry[T any] struct {
	key   string
	value T
}

// PreparedCache is a bounded concurrent LRU for immutable native catalogs.
type PreparedCache[T any] struct {
	mu         sync.Mutex
	capacity   int
	items      map[string]*list.Element
	order      *list.List
	group      singleflight.Group
	generation uint64
}

func NewPreparedCache[T any](capacity int) *PreparedCache[T] {
	if capacity < 1 {
		capacity = 1
	}
	return &PreparedCache[T]{
		capacity: capacity,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

func (c *PreparedCache[T]) GetOrBuild(ctx context.Context, key string, build func() (T, error)) (T, error) {
	if value, ok := c.get(key); ok {
		return value, nil
	}
	value, err, _ := c.group.Do(key, func() (any, error) {
		if value, ok := c.get(key); ok {
			return value, nil
		}
		generation := c.currentGeneration()
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		value, err := build()
		if err != nil {
			var zero T
			return zero, err
		}
		c.setIfGeneration(key, value, generation)
		return value, nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}

func (c *PreparedCache[T]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	for key, element := range c.items {
		if strings.HasPrefix(key, prefix) {
			c.order.Remove(element)
			delete(c.items, key)
		}
	}
}

func (c *PreparedCache[T]) get(key string) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	element, ok := c.items[key]
	if !ok {
		var zero T
		return zero, false
	}
	c.order.MoveToFront(element)
	return element.Value.(preparedEntry[T]).value, true
}

func (c *PreparedCache[T]) currentGeneration() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation
}

func (c *PreparedCache[T]) setIfGeneration(key string, value T, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation {
		return
	}
	if element, ok := c.items[key]; ok {
		element.Value = preparedEntry[T]{key: key, value: value}
		c.order.MoveToFront(element)
		return
	}
	c.items[key] = c.order.PushFront(preparedEntry[T]{key: key, value: value})
	for c.order.Len() > c.capacity {
		element := c.order.Back()
		entry := element.Value.(preparedEntry[T])
		delete(c.items, entry.key)
		c.order.Remove(element)
	}
}
