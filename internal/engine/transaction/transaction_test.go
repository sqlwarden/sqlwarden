package transaction

import "testing"

func TestNewSavepointName(t *testing.T) {
	if got := NewSavepointName(3); got != "sqlwarden_sp_3" {
		t.Fatalf("NewSavepointName(3) = %q, want %q", got, "sqlwarden_sp_3")
	}
}
