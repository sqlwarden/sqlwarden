// Copyright (c) SQLWarden. All rights reserved.
// Licensed under the SQLWarden Enterprise License. See enterprise/LICENSE.

package ee

import (
	"fmt"

	"github.com/sqlwarden/internal/license"
)

// placeholderLicense reports the enterprise edition with nothing licensed.
// Real Ed25519 license-key verification replaces this in a later phase; an
// unlicensed enterprise binary must behave exactly like community.
type placeholderLicense struct{}

func (placeholderLicense) Edition() string            { return "enterprise" }
func (placeholderLicense) IsLicensed(string) bool     { return false }
func (placeholderLicense) LicensedFeatures() []string { return nil }

func (placeholderLicense) Require(feature string) error {
	return fmt.Errorf("%s: %w", feature, license.ErrNotLicensed)
}
