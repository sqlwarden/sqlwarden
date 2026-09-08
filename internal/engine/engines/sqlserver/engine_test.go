package sqlserver

import (
	"testing"

	"github.com/sqlwarden/internal/engine/enginetest"
)

func TestCapabilityContract(t *testing.T) {
	enginetest.RunCapabilityContract(t, "sqlserver")
}
