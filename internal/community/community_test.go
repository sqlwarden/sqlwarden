package community

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/edition/editiontest"
)

func TestCommunityEditionContract(t *testing.T) {
	editiontest.Run(t, New(), edition.Dependencies{})
	composition, err := edition.Compose(context.Background(), New(), edition.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if composition.Name() != edition.CommunityName || composition.Capabilities().Features[0].State != edition.StateUpgrade {
		t.Fatalf("Community capabilities = %+v", composition.Capabilities())
	}
}
