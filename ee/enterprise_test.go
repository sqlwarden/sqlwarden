//go:build enterprise

package ee

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/sqlwarden/internal/edition"
	"github.com/sqlwarden/internal/edition/editiontest"
)

func TestEnterpriseEditionContract(t *testing.T) {
	editiontest.Run(t, New(), edition.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	composition, err := edition.Compose(context.Background(), New(), edition.Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	if composition.Name() != edition.EnterpriseName || composition.Capabilities().Features[0].State != edition.StateAvailable {
		t.Fatalf("Enterprise capabilities = %+v", composition.Capabilities())
	}
}
