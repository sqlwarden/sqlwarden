// Package license defines the edition/licensing seam. The community
// implementation reports no licensed features and handles no keys;
// enterprise builds provide their own Service through the extension
// registry's single validated license factory.
package license

import (
	"errors"
	"fmt"
)

var ErrNotLicensed = errors.New("feature requires an enterprise license")

// CodeRequired is the API error envelope code every license-gated endpoint
// returns when the feature is not licensed.
const CodeRequired = "enterprise_license_required"

type Service interface {
	Edition() string
	IsLicensed(feature string) bool
	LicensedFeatures() []string
	Require(feature string) error
}

type communityService struct{}

func Community() Service { return communityService{} }

func (communityService) Edition() string            { return "community" }
func (communityService) IsLicensed(string) bool     { return false }
func (communityService) LicensedFeatures() []string { return nil }

func (communityService) Require(feature string) error {
	return fmt.Errorf("%s: %w", feature, ErrNotLicensed)
}
