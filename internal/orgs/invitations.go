package orgs

// InvitationPolicy reports whether organization invitations are available.
type InvitationPolicy interface {
	Enabled() bool
}

type invitationPolicy bool

func (p invitationPolicy) Enabled() bool { return bool(p) }

var (
	InvitationsEnabled  InvitationPolicy = invitationPolicy(true)
	InvitationsDisabled InvitationPolicy = invitationPolicy(false)
)
