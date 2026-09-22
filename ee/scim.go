package ee

import (
	"fmt"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
)

// CapabilitySCIM identifies directory provisioning support in capability
// responses. It never substitutes for backend authorization.
const CapabilitySCIM = "identity.scim"

type scimModule struct{}

func (scimModule) Name() string { return "scim" }

func (scimModule) Capabilities() edition.Entitlements {
	return edition.Entitlements{CapabilitySCIM: true}
}

func (scimModule) Validate(cfg config.Config) error {
	if cfg.Edition.Name != config.EditionEnterprise {
		return fmt.Errorf("SCIM module requires enterprise edition")
	}
	if cfg.Edition.LicenseFile == "" {
		return fmt.Errorf("SCIM module requires an enterprise license source")
	}
	return nil
}

var _ edition.Module = scimModule{}
