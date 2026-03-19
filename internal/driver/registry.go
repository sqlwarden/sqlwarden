package driver

import (
	"fmt"
	"sync"
)

var (
	mu       sync.RWMutex
	registry = map[string]func() Driver{}
)

// Register registers a driver factory under the given name.
// Panics if name is empty or already registered.
func Register(name string, factory func() Driver) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("driver: Register called with empty name")
	}
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("driver: Register called twice for driver %q", name))
	}
	registry[name] = factory
}

// New returns a new Driver instance for the given name.
// Returns an error if the driver is not registered.
// Normalizes aliases: "postgresql" → "postgres", "sqlite3" → "sqlite", "mariadb" → "mysql".
func New(name string) (Driver, error) {
	name = normalizeAlias(name)
	mu.RLock()
	factory, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("driver: unknown driver %q", name)
	}
	return factory(), nil
}

func normalizeAlias(name string) string {
	switch name {
	case "postgresql":
		return "postgres"
	case "sqlite3":
		return "sqlite"
	case "mariadb":
		return "mysql"
	default:
		return name
	}
}
