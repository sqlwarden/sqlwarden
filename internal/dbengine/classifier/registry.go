package classifier

import (
	"sync"

	"github.com/sqlwarden/internal/driver"
)

var (
	mu       sync.RWMutex
	registry = map[driver.Dialect]Classifier{}
)

// Register installs a dialect-specific classifier. Called from driver init.
func Register(d driver.Dialect, c Classifier) {
	mu.Lock()
	defer mu.Unlock()
	registry[d] = c
}

// For returns the classifier registered for d, falling back to the dialect-
// agnostic heuristic so callers always get a usable classifier (never nil).
func For(d driver.Dialect) Classifier {
	mu.RLock()
	c, ok := registry[d]
	mu.RUnlock()
	if ok && c != nil {
		return c
	}
	return NewHeuristic()
}

// unregister is a test helper.
func unregister(d driver.Dialect) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, d)
}
