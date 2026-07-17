// Package capability defines availability checks for optional application
// capabilities. Core does not know why a capability is unavailable; build
// composition, deployment configuration, or an extension may decide that.
package capability

import (
	"errors"
	"fmt"
)

var ErrUnavailable = errors.New("capability is unavailable")

// CodeUnavailable is the stable API error code returned when an optional
// capability cannot be used in the current deployment.
const CodeUnavailable = "capability_unavailable"

// Gate is the runtime availability boundary for optional contributions.
type Gate interface {
	Enabled(capability string) bool
	EnabledCapabilities() []string
	Require(capability string) error
}

type unavailableGate struct{}

// None returns a gate with no enabled optional capabilities.
func None() Gate { return unavailableGate{} }

func (unavailableGate) Enabled(string) bool           { return false }
func (unavailableGate) EnabledCapabilities() []string { return nil }
func (unavailableGate) Require(name string) error     { return fmt.Errorf("%s: %w", name, ErrUnavailable) }
