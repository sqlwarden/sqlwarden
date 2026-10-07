package access

import (
	"testing"
	"time"
)

func TestSignalValid(t *testing.T) {
	now := time.Now()
	var unset *Signal[bool]
	if unset.Valid(now) {
		t.Fatal("nil signal is valid")
	}
	if !(&Signal[bool]{Value: true}).Valid(now) {
		t.Fatal("signal without expiry is invalid")
	}
	if (&Signal[bool]{ExpiresAt: now.Add(-time.Second)}).Valid(now) {
		t.Fatal("expired signal is valid")
	}
}

func TestDecisionHelpers(t *testing.T) {
	if Allow().Effect != EffectAllow {
		t.Fatal("Allow")
	}
	if d := Deny("ip_blocked"); d.Effect != EffectDeny || d.Reason != "ip_blocked" {
		t.Fatalf("Deny = %+v", d)
	}
}
