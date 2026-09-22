package catalog

import (
	"context"
	"strconv"
	"strings"

	"github.com/sqlwarden/internal/audit"
)

// Audited actions emitted by catalog use cases. Metadata carries identifiers
// and decisions only: no DSNs, credentials, SQL, tokens, or row values.
const (
	ActionOrgCreated            = "catalog.organization.created"
	ActionOrgUpdated            = "catalog.organization.updated"
	ActionOrgDeleted            = "catalog.organization.deleted"
	ActionOrgMemberRemoved      = "catalog.organization.member_removed"
	ActionOrgMemberRoleUpdated  = "catalog.organization.member_role_updated"
	ActionWorkspaceCreated      = "catalog.workspace.created"
	ActionWorkspaceUpdated      = "catalog.workspace.updated"
	ActionWorkspaceDeleted      = "catalog.workspace.deleted"
	ActionEnvironmentCreated    = "catalog.environment.created"
	ActionEnvironmentUpdated    = "catalog.environment.updated"
	ActionEnvironmentDeleted    = "catalog.environment.deleted"
	ActionConnectionCreated     = "catalog.connection.created"
	ActionConnectionUpdated     = "catalog.connection.updated"
	ActionConnectionDeleted     = "catalog.connection.deleted"
	ActionConnectionDSNRevealed = "catalog.connection.dsn_revealed"

	resourceOrganization = "organization"
	resourceWorkspace    = "workspace"
	resourceEnvironment  = "environment"
	resourceConnection   = "connection"
)

// Actor is the account a catalog use case runs on behalf of, and the
// organization the request was scoped to. Both are recorded on the audit trail
// so a mutation can always be attributed to a principal and a tenant.
type Actor struct {
	AccountID int64
	OrgID     int64
}

// Service is the application service for the resource catalog. It owns
// validation, ownership checks, hierarchy and policy seeding, authorization
// cache invalidation, credential sealing, and connection lifecycle rules for
// organizations, workspaces, environments, and connections.
type Service struct {
	store    Store
	grants   Grants
	sessions Sessions
	sealer   Sealer
	targets  *TargetPolicy
	audit    audit.Writer
}

// NewService returns the catalog service. The writer is the edition-composed
// audit sink every mutating use case emits through; a nil writer records
// nothing rather than panicking.
func NewService(store Store, grants Grants, sessions Sessions, sealer Sealer, targets *TargetPolicy, writer audit.Writer) *Service {
	if writer == nil {
		writer = audit.Discard
	}
	return &Service{store: store, grants: grants, sessions: sessions, sealer: sealer, targets: targets, audit: writer}
}

// Targets exposes the instance policy governing which target databases may be
// registered. Transports that validate a target outside a catalog use case,
// such as an ad-hoc connection test, share this policy rather than restating
// it.
func (s *Service) Targets() *TargetPolicy { return s.targets }

// auditSuccess records a completed catalog mutation. Its error is returned to
// the caller: a catalog change that could not be audited is not reported as
// having succeeded.
func (s *Service) auditSuccess(ctx context.Context, actor Actor, action, resource string, resourceID int64, metadata map[string]string) error {
	return audit.Emit(ctx, s.audit, s.event(actor, action, audit.OutcomeSuccess, resource, resourceID, metadata))
}

// auditDenied records a refused catalog mutation and returns cause unchanged,
// so an audit sink problem cannot mask why the action was refused.
func (s *Service) auditDenied(ctx context.Context, actor Actor, action, resource string, resourceID int64, metadata map[string]string, cause error) error {
	_ = audit.Emit(ctx, s.audit, s.event(actor, action, audit.OutcomeDenied, resource, resourceID, metadata))
	return cause
}

func (s *Service) event(actor Actor, action, outcome, resource string, resourceID int64, metadata map[string]string) audit.Event {
	event := audit.Event{
		Action:   action,
		Resource: resource,
		Outcome:  outcome,
		Metadata: metadata,
	}
	if actor.AccountID != 0 {
		accountID := actor.AccountID
		event.AccountID = &accountID
	}
	if actor.OrgID != 0 {
		orgID := actor.OrgID
		event.OrgID = &orgID
	}
	if resourceID != 0 {
		event.ResourceID = strconv.FormatInt(resourceID, 10)
	}
	return event
}

// ListQuery is the shared filtering, sorting, and pagination request for
// catalog list use cases. The catalog sorts and pages in memory because the
// underlying reads already narrow rows to what the account may see.
type ListQuery struct {
	Search   string
	Name     string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

func trimmed(s string) string { return strings.TrimSpace(s) }

func matchesSearch(value, search string) bool {
	return search == "" || strings.Contains(strings.ToLower(value), search)
}
