// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise Source License. See enterprise/LICENSE.

package ee

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/capability"
	"github.com/sqlwarden/internal/extension"
)

// placeholderLicense reports the enterprise edition with nothing licensed.
// Real Ed25519 license-key verification replaces this in a later phase; an
// unlicensed enterprise binary must behave exactly like community.
type placeholderLicense struct{}

func newCapabilityGate(context.Context, extension.BootstrapDeps) (capability.Gate, error) {
	return placeholderLicense{}, nil
}

func (placeholderLicense) Enabled(string) bool           { return false }
func (placeholderLicense) EnabledCapabilities() []string { return nil }

func (placeholderLicense) Require(name string) error {
	return fmt.Errorf("%s: %w", name, capability.ErrUnavailable)
}
