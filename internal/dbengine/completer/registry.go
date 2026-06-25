package completer

import (
	"sync"

	"github.com/sqlwarden/internal/driver"
)

var (
	mu       sync.RWMutex
	registry = map[driver.Dialect]Completer{}
)

// Register installs a dialect-specific completer. Called from driver init.
func Register(d driver.Dialect, c Completer) {
	mu.Lock()
	defer mu.Unlock()
	registry[d] = c
}

// For returns the completer registered for d, or nil if none.
func For(d driver.Dialect) Completer {
	mu.RLock()
	defer mu.RUnlock()
	return registry[d]
}
