//go:build enterprise

package main

import (
	"testing"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
)

func TestSelectedEnterpriseEditionAcceptsLicense(t *testing.T) {
	selected, err := selectedEdition(config.Config{License: "future-license-material"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name() != edition.EnterpriseName {
		t.Fatalf("edition = %q", selected.Name())
	}
}
