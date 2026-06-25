package rewriter

import (
	"sync"

	"github.com/sqlwarden/internal/driver"
)

var (
	mu       sync.RWMutex
	registry = map[driver.Dialect]Rewriter{}
)

// Register installs a dialect-specific rewriter. Called from driver init.
func Register(d driver.Dialect, rw Rewriter) {
	mu.Lock()
	defer mu.Unlock()
	registry[d] = rw
}

// For returns the rewriter registered for d, or nil if none.
func For(d driver.Dialect) Rewriter {
	mu.RLock()
	defer mu.RUnlock()
	return registry[d]
}
