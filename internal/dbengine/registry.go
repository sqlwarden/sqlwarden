package dbengine

import (
	"fmt"
	"sort"
	"sync"

	"github.com/sqlwarden/internal/driver"
)

// Registration declares one engine: its identity plus a factory that returns a
// fresh, non-connected driver. The driver is the unit — call Connect on it for
// a live session, or assert capability interfaces directly for connectionless
// features (classification, parsing, …).
type Registration struct {
	ID          EngineID
	DisplayName string
	Dialect     Dialect
	NewDriver   func() driver.Driver
}

var (
	registryMu sync.RWMutex
	registry   = map[EngineID]Registration{}
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
	registry[reg.ID] = reg
}

// New returns a fresh, non-connected driver for the engine registered under
// name (alias-normalized: "postgresql" -> "postgres", etc.). Call Connect for a
// live session; assert capability interfaces for connectionless features.
func New(name string) (driver.Driver, error) {
	registryMu.RLock()
	reg, ok := registry[EngineID(driver.NormalizeName(name))]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownEngine, name)
	}
	return reg.NewDriver(), nil
}

// Describe returns the static capability report for one engine.
func Describe(name string) (CapabilitySet, bool) {
	registryMu.RLock()
	reg, ok := registry[EngineID(driver.NormalizeName(name))]
	registryMu.RUnlock()
	if !ok {
		return CapabilitySet{}, false
	}
	return capabilityReport(reg), true
}

// Engines returns the static capability report for every registered engine,
// sorted by id for stable output.
func Engines() []CapabilitySet {
	registryMu.RLock()
	regs := make([]Registration, 0, len(registry))
	for _, reg := range registry {
		regs = append(regs, reg)
	}
	registryMu.RUnlock()
	sort.Slice(regs, func(i, j int) bool { return regs[i].ID < regs[j].ID })

	out := make([]CapabilitySet, 0, len(regs))
	for _, reg := range regs {
		out = append(out, capabilityReport(reg))
	}
	return out
}
