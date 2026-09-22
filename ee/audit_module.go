package ee

import (
	"fmt"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
)

// auditModule declares the tamper-evidence capability. The tables it uses
// belong to the shared Enterprise migration stream, so the module contributes
// no stream of its own.
type auditModule struct{}

func (auditModule) Name() string { return "audit" }

func (auditModule) Capabilities() edition.Entitlements {
	return edition.Entitlements{CapabilityAuditTamperEvidence: true}
}

func (auditModule) Validate(cfg config.Config) error {
	if cfg.Edition.Name != config.EditionEnterprise {
		return fmt.Errorf("tamper-evident audit requires enterprise edition")
	}
	if cfg.Edition.LicenseFile == "" {
		return fmt.Errorf("tamper-evident audit requires an enterprise license source")
	}
	return nil
}

var _ edition.Module = auditModule{}
