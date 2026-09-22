package community

import (
	"testing"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/edition/editiontest"
)

func TestEditionContract(t *testing.T) {
	editiontest.Run(t, edition.NewCommunity(), config.Default(), edition.Dependencies{}.Normalize())
}
