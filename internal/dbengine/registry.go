package dbengine

import (
	"fmt"
	"sort"
	"sync"

	"github.com/sqlwarden/internal/driver"
)

// Registration declares one engine by composing the existing driver factory.
// During the facade phase the engine reuses the registered driver type; later
// phases replace NewDriver with an engine-native connection opener without
// changing this contract.
type Registration struct {
	ID          EngineID
	DisplayName string
	Dialect     Dialect
	NewDriver   func() driver.Driver
}

var (
	registryMu sync.RWMutex
	registry   = map[EngineID]*facadeEngine{}
)

// Register installs an engine. Panics on empty id, nil factory, or duplicate id.
func Register(reg Registration) {
	if reg.ID == "" {
		panic("dbengine: Register called with empty id")
	}
	if reg.NewDriver == nil {
		panic(fmt.Sprintf("dbengine: Register %q called with nil NewDriver", reg.ID))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[reg.ID]; exists {
		panic(fmt.Sprintf("dbengine: Register called twice for engine %q", reg.ID))
	}
	registry[reg.ID] = &facadeEngine{reg: reg}
}

// New returns the engine registered for id, normalizing known aliases
// ("postgresql" -> "postgres", etc.) via the shared driver alias table.
func New(id string) (Engine, error) {
	normalized := EngineID(driver.NormalizeName(id))
	registryMu.RLock()
	eng, ok := registry[normalized]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEngine, id)
	}
	return eng, nil
}

// Engines returns all registered engines sorted by id for stable output.
func Engines() []Engine {
	registryMu.RLock()
	out := make([]Engine, 0, len(registry))
	for _, eng := range registry {
		out = append(out, eng)
	}
	registryMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}
