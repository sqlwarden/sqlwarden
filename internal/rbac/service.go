package rbac

import (
	"context"
	"io"
	"log/slog"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
)

const (
	ActionRoleCreated   = "role.created"
	ActionRoleUpdated   = "role.updated"
	ActionRoleDeleted   = "role.deleted"
	ActionPolicyGranted = "policy.granted"
	ActionPolicyRevoked = "policy.revoked"
)

type Deps struct {
	Store    Store
	Requirer *access.Requirer
	// Policy resolves effective permissions. It must be the evaluator the
	// Requirer authorizes with, so reported and enforced permissions agree.
	Policy access.PolicyEvaluator
	// Audit defaults to audit.Discard and Logger to a discarding logger.
	Audit  audit.Writer
	Logger *slog.Logger
}

// Service administers roles and policy bindings. Every method authorizes the
// principal itself, so callers need no prior permission check.
type Service struct {
	store    Store
	requirer *access.Requirer
	policy   access.PolicyEvaluator
	audit    audit.Writer
	logger   *slog.Logger
}

func New(deps Deps) *Service {
	if deps.Audit == nil {
		deps.Audit = audit.Discard
	}
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		store:    deps.Store,
		requirer: deps.Requirer,
		policy:   deps.Policy,
		audit:    deps.Audit,
		logger:   deps.Logger,
	}
}

// requireOrg admits org members who hold permission on the organization. An
// empty permission requires membership only.
func (s *Service) requireOrg(ctx context.Context, p access.Principal, orgID int64, permission string) error {
	if _, err := s.requirer.Org(ctx, p, orgID); err != nil {
		return err
	}
	if permission != "" && !s.requirer.Can(ctx, p, orgID, "org", "org", orgID, permission) {
		return ErrForbidden
	}
	return nil
}

// requireWorkspace admits org members when the workspace belongs to the
// organization and they hold permission on it. An empty permission requires
// membership only.
func (s *Service) requireWorkspace(ctx context.Context, p access.Principal, orgID, workspaceID int64, permission string) error {
	_, err := s.requirer.Workspace(ctx, p, orgID, workspaceID, permission)
	return err
}

// emit records a success event on a best-effort basis; a failed audit write
// must not undo or fail a change that already committed.
func (s *Service) emit(ctx context.Context, p access.Principal, orgID int64, action, resource string, resourceID int64, metadata map[string]string) {
	err := audit.Emit(ctx, s.audit, audit.Event{
		OrgID:      &orgID,
		Actor:      audit.ActorFromPrincipal(p),
		Action:     action,
		Resource:   resource,
		ResourceID: strconv.FormatInt(resourceID, 10),
		Outcome:    audit.OutcomeSuccess,
		Metadata:   metadata,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "rbac audit failed", slog.String("action", action), slog.Int64("org_id", orgID), slog.Any("error", err))
	}
}

// addField keeps the first message reported for a field.
func addField(fields map[string]string, field, message string) {
	if _, exists := fields[field]; !exists {
		fields[field] = message
	}
}
