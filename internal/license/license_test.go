package license

import (
	"errors"
	"testing"
)

func TestCommunityService(t *testing.T) {
	svc := Community()

	if got := svc.Edition(); got != "community" {
		t.Fatalf("Edition() = %q, want community", got)
	}
	if svc.IsLicensed("audit_log") {
		t.Fatal("community edition must not license any feature")
	}
	if feats := svc.LicensedFeatures(); len(feats) != 0 {
		t.Fatalf("LicensedFeatures() = %v, want empty", feats)
	}
	err := svc.Require("audit_log")
	if !errors.Is(err, ErrNotLicensed) {
		t.Fatalf("Require() error = %v, want ErrNotLicensed", err)
	}
}
