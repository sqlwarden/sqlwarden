package orgs

import "testing"

func TestInvitationPolicies(t *testing.T) {
	if !InvitationsEnabled.Enabled() {
		t.Fatal("InvitationsEnabled.Enabled() = false")
	}
	if InvitationsDisabled.Enabled() {
		t.Fatal("InvitationsDisabled.Enabled() = true")
	}
}
