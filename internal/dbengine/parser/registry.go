package parser

import (
	"sync"

	"github.com/sqlwarden/internal/driver"
)

var (
	mu       sync.RWMutex
	registry = map[driver.Dialect]Parser{}
)

// Register installs a dialect-specific parser. Called from driver init.
func Register(d driver.Dialect, p Parser) {
	mu.Lock()
	defer mu.Unlock()
	registry[d] = p
}

// For returns the parser registered for d, or nil if none.
func For(d driver.Dialect) Parser {
	mu.RLock()
	defer mu.RUnlock()
	return registry[d]
}
