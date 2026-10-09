package catalog

import (
	"context"
	"errors"
	"slices"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/credentials"
)

const actionConnectionSecretRevealed = "connection.secret_revealed"

func (s *Service) RevealSecret(ctx context.Context, p access.Principal, ref ConnRef, name credentials.SecretName) (string, error) {
	if s.deps.Audit == nil {
		return "", errors.New("catalog: audit writer is not configured")
	}
	_, ws, err := s.loadConnection(ctx, p, ref, access.PermConnRead)
	if err != nil {
		return "", err
	}
	if !slices.Contains(allSecretNames[:], name) {
		return "", validationError("secret", "Secret name is not supported.")
	}
	state, err := s.secretState(ctx, ref, name)
	if err != nil {
		return "", err
	}
	if !state.Set || state.Source != credentials.SourceStored {
		return "", &ErrReveal{Code: RevealCodeNotRevealable}
	}
	allowed, err := s.revealPolicyAllowed(ctx, p, ref.OrgID)
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", &ErrReveal{Code: RevealCodeDisabled}
	}
	if s.deps.Policy == nil || !s.deps.Policy.Can(ctx, p.Subject.ID, ref.OrgID, ws.OwnerType, "connection", ref.ConnectionID, access.PermConnRevealSecret) {
		return "", ErrForbidden
	}
	value, err := s.deps.Credentials.Reveal(ctx, credentialsRef(ref), name)
	if err != nil {
		if errors.Is(err, credentials.ErrNotRevealable) {
			return "", &ErrReveal{Code: RevealCodeNotRevealable}
		}
		return "", normalizeCredentialError(err)
	}
	if err := s.emitSecretRevealedAudit(ctx, p, ref, name, ""); err != nil {
		return "", err
	}
	s.deps.Logger.Info("connection secret revealed", "connection_id", ref.ConnectionID, "secret_name", string(name), "principal_id", p.Subject.ID)
	return value, nil
}

func (s *Service) emitSecretRevealedAudit(ctx context.Context, p access.Principal, ref ConnRef, name credentials.SecretName, via string) error {
	if s.deps.Audit == nil {
		return errors.New("catalog: audit writer is not configured")
	}
	metadata := map[string]string{"secret_name": string(name), "workspace_id": strconv.FormatInt(ref.WorkspaceID, 10)}
	if via != "" {
		metadata["via"] = via
	}
	orgID := ref.OrgID
	return audit.Emit(ctx, s.deps.Audit, audit.Event{
		OrgID:      &orgID,
		Actor:      audit.ActorFromPrincipal(p),
		Action:     actionConnectionSecretRevealed,
		Resource:   "connection",
		ResourceID: strconv.FormatInt(ref.ConnectionID, 10),
		Outcome:    audit.OutcomeSuccess,
		Metadata:   metadata,
	})
}

// Revealable evaluates every reveal gate except opening the secret value.
func (s *Service) Revealable(ctx context.Context, p access.Principal, ref ConnRef, name credentials.SecretName) (bool, error) {
	_, ws, err := s.loadConnection(ctx, p, ref, access.PermConnRead)
	if err != nil {
		return false, err
	}
	state, err := s.secretState(ctx, ref, name)
	if err != nil {
		return false, err
	}
	gate := &revealGate{service: s, principal: p, ref: ref, ownerType: ws.OwnerType}
	return gate.revealable(ctx, state)
}

func (s *Service) secretState(ctx context.Context, ref ConnRef, name credentials.SecretName) (credentials.SecretState, error) {
	if s.deps.Credentials == nil {
		return credentials.SecretState{}, errors.New("catalog: credential provider is not configured")
	}
	states, err := s.deps.Credentials.Describe(ctx, credentialsRef(ref))
	if err != nil {
		return credentials.SecretState{}, normalizeCredentialError(err)
	}
	state, ok := states[name]
	if !ok {
		return credentials.SecretState{}, &ErrReveal{Code: RevealCodeNotRevealable}
	}
	return state, nil
}

// revealGate evaluates the connection-wide reveal gates (org policy and the
// reveal permission) at most once, however many secrets are inspected.
type revealGate struct {
	service   *Service
	principal access.Principal
	ref       ConnRef
	ownerType string
	evaluated bool
	allowed   bool
	err       error
}

func (g *revealGate) revealable(ctx context.Context, state credentials.SecretState) (bool, error) {
	if !state.Set || state.Source != credentials.SourceStored {
		return false, nil
	}
	if !g.evaluated {
		g.evaluated = true
		g.allowed, g.err = g.evaluate(ctx)
	}
	return g.allowed, g.err
}

func (g *revealGate) evaluate(ctx context.Context) (bool, error) {
	s := g.service
	allowed, err := s.revealPolicyAllowed(ctx, g.principal, g.ref.OrgID)
	if err != nil || !allowed {
		return false, err
	}
	if s.deps.Policy == nil {
		return false, nil
	}
	return s.deps.Policy.Can(ctx, g.principal.Subject.ID, g.ref.OrgID, g.ownerType, "connection", g.ref.ConnectionID, access.PermConnRevealSecret), nil
}

func (s *Service) revealPolicyAllowed(ctx context.Context, p access.Principal, orgID int64) (bool, error) {
	if s.deps.RevealPolicy == nil {
		return false, errors.New("catalog: reveal policy is not configured")
	}
	return s.deps.RevealPolicy.Allowed(ctx, credentials.OrgRef{OrgID: strconv.FormatInt(orgID, 10)}, p)
}
