package capability

import (
	"errors"
	"testing"
)

func TestUnavailableGate(t *testing.T) {
	gate := None()

	if gate.Enabled("audit_log") {
		t.Fatal("empty capability gate must not enable optional capabilities")
	}
	if capabilities := gate.EnabledCapabilities(); len(capabilities) != 0 {
		t.Fatalf("EnabledCapabilities() = %v, want empty", capabilities)
	}
	if err := gate.Require("audit_log"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Require() error = %v, want ErrUnavailable", err)
	}
}
