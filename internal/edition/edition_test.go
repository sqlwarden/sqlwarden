//go:build !enterprise

package edition

import "testing"

func TestCommunityRegistryIsEmpty(t *testing.T) {
	if got := Registry().All(); len(got) != 0 {
		t.Fatalf("community registry must be empty, got %d extensions", len(got))
	}
}
