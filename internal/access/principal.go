package access

import (
	"context"
	"net/netip"
	"time"
)

// SubjectKind identifies the kind of principal subject.
type SubjectKind string

const (
	SubjectAccount        SubjectKind = "account"
	SubjectTeam           SubjectKind = "team"
	SubjectServiceAccount SubjectKind = "service_account"
)

// SubjectRef identifies a principal subject.
type SubjectRef struct {
	Kind SubjectKind
	ID   int64
}

// CredentialKind identifies the kind of credential used to authenticate.
type CredentialKind string

const (
	CredentialSession    CredentialKind = "session"
	CredentialAPIKey     CredentialKind = "api_key"
	CredentialOAuthToken CredentialKind = "oauth_token"
	CredentialSCIMToken  CredentialKind = "scim_token"
	CredentialLocal      CredentialKind = "local"
)

// AAL identifies an authentication assurance level.
type AAL string

const (
	AAL1 AAL = "aal1"
	AAL2 AAL = "aal2"
	AAL3 AAL = "aal3"
)

// CredentialInfo describes the credential used to authenticate a principal.
type CredentialInfo struct {
	Kind      CredentialKind
	ID        string
	Method    string
	AuthTime  time.Time
	Assurance AAL
	ClientID  string
}

// Signal is an observed request signal with provenance and validity metadata.
type Signal[T any] struct {
	Value      T
	Source     string
	ObservedAt time.Time
	ExpiresAt  time.Time
}

// Valid reports whether the signal is set and not expired at now.
func (s *Signal[T]) Valid(now time.Time) bool {
	return s != nil && (s.ExpiresAt.IsZero() || now.Before(s.ExpiresAt))
}

// Signals contains posture signals collected for a request.
type Signals struct {
	DeviceID      *Signal[string]
	DeviceManaged *Signal[bool]
	MFA           *Signal[bool]
	Geo           *Signal[string]
}

// RequestAttributes describes the request being authenticated.
type RequestAttributes struct {
	ClientIP   netip.Addr
	UserAgent  string
	ClientKind string
	Time       time.Time
	Signals    Signals
}

// ScopeSet limits the permissions available to a principal.
type ScopeSet struct {
	Permissions []string
}

// Principal is the authenticated identity and its request context.
type Principal struct {
	Subject       SubjectRef
	OnBehalfOf    *SubjectRef
	Credential    CredentialInfo
	Ceiling       *ScopeSet
	OrgBinding    *int64
	InstanceAdmin bool
	Request       RequestAttributes
}

// Effect identifies the outcome of an access decision.
type Effect string

const (
	EffectAllow  Effect = "allow"
	EffectDeny   Effect = "deny"
	EffectStepUp Effect = "step_up"
)

// Requirement describes additional authentication required for access.
type Requirement struct {
	Assurance AAL
	Methods   []string
}

// Decision is the outcome of an access evaluation.
type Decision struct {
	Effect Effect
	Reason string
	StepUp *Requirement
}

// Allow returns an allow decision.
func Allow() Decision { return Decision{Effect: EffectAllow} }

// Deny returns a deny decision with a stable reason code.
func Deny(reason string) Decision { return Decision{Effect: EffectDeny, Reason: reason} }

// Target identifies the resource being evaluated.
type Target struct {
	Kind string
	ID   int64
}

// Condition validates and evaluates a conditional access rule.
type Condition interface {
	Kind() string
	Validate(params map[string]string) error
	Evaluate(ctx context.Context, p Principal, t Target, params map[string]string) Decision
}
