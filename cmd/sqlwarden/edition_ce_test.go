//go:build !enterprise

package main

import (
	"testing"

	"github.com/sqlwarden/internal/config"
	"github.com/sqlwarden/internal/edition"
)

func TestSelectedCommunityEditionRejectsLicense(t *testing.T) {
	if _, err := selectedEdition(config.Config{License: "present"}); err == nil || err.Error() != "the license setting requires the enterprise build" {
		t.Fatalf("error = %v", err)
	}
	selected, err := selectedEdition(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name() != edition.CommunityName {
		t.Fatalf("edition = %q", selected.Name())
	}
}
