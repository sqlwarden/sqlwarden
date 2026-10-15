package orgs

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
)

// Org is an organization as the service returns it.
type Org = database.Organization

const (
	ActionOrgCreated        = "org.created"
	ActionOrgUpdated        = "org.updated"
	ActionOrgDeleted        = "org.deleted"
	ActionMemberRemoved     = "org.member_removed"
	ActionMemberRoleChanged = "org.member_role_changed"
)

type Deps struct {
	Store    Store
	Teams    TeamStore
	Requirer *access.Requirer
	// Cache drops cached policy after membership and binding changes. It may
	// be nil when no policy is cached.
	Cache   PolicyCache
	Revoker SessionRevoker
	// Invitations stores organization invitations. InvitationPolicy gates
	// them and defaults to InvitationsDisabled. A nil Mailer reports every
	// delivery as disabled.
	Invitations      InvitationStore
	InvitationPolicy InvitationPolicy
	Mailer           Mailer
	BaseURL          BaseURLSource
	// Audit defaults to audit.Discard and Logger to a discarding logger.
	Audit  audit.Writer
	Logger *slog.Logger
}

// Service owns organizations and their membership. Every method authorizes
// the principal itself, so callers need no prior permission check.
type Service struct {
	store    Store
	teams    TeamStore
	requirer *access.Requirer
	cache    PolicyCache
	revoker  SessionRevoker
	audit    audit.Writer
	logger   *slog.Logger

	invitations      InvitationStore
	invitationPolicy InvitationPolicy
	mailer           Mailer
	baseURL          BaseURLSource

	mu             sync.RWMutex
	purgeObservers []func(connectionID int64)
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
		teams:    deps.Teams,
		requirer: deps.Requirer,
		cache:    deps.Cache,
		revoker:  deps.Revoker,
		audit:    deps.Audit,
		logger:   deps.Logger,

		invitations:      deps.Invitations,
		invitationPolicy: deps.InvitationPolicy,
		mailer:           deps.Mailer,
		baseURL:          deps.BaseURL,
	}
}

// OnSchemaCachePurged registers fn to observe every connection whose stored
// schema cache the service purges, so in-memory caches can drop it too.
func (s *Service) OnSchemaCachePurged(fn func(connectionID int64)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeObservers = append(s.purgeObservers, fn)
}

type CreateOrgInput struct {
	Name string
	// Slug is derived from Name when empty.
	Slug string
}

// Create creates an organization owned by the calling account. Only instance
// admins may create organizations.
func (s *Service) Create(ctx context.Context, p access.Principal, in CreateOrgInput) (Org, error) {
	if err := requireInstanceAdmin(p); err != nil {
		return Org{}, err
	}

	name := strings.TrimSpace(in.Name)
	explicitSlug := strings.TrimSpace(in.Slug)
	fields := map[string]string{}
	if name == "" {
		addField(fields, "name", "Name is required.")
	}
	slug := explicitSlug
	if slug == "" {
		slug = Slugify(name)
	}
	if slug == "" {
		addField(fields, "slug", "Slug is required.")
	} else {
		if !ValidSlug(slug) {
			addField(fields, "slug", "Slug may only contain lowercase letters, numbers, and hyphens.")
		}
		if len(slug) > MaxSlugLength {
			addField(fields, "slug", "Slug must be 64 characters or fewer.")
		}
	}
	if len(fields) != 0 {
		return Org{}, &ValidationError{Fields: fields}
	}

	org, err := s.store.CreateOwnedOrg(ctx, slug, name, p.Subject.ID)
	if err != nil {
		if isUniqueViolation(err) {
			if explicitSlug != "" {
				return Org{}, &ValidationError{Fields: map[string]string{"slug": "An organization with this slug already exists."}}
			}
			return Org{}, &ValidationError{Fields: map[string]string{"name": "An organization with this name already exists."}}
		}
		return Org{}, err
	}
	s.invalidateOrgPolicy(org.ID)
	s.emit(ctx, p, org.ID, ActionOrgCreated, "org", org.ID, nil)
	return org, nil
}

// Get returns the organization to any of its members.
func (s *Service) Get(ctx context.Context, p access.Principal, orgID int64) (Org, error) {
	if _, err := s.requirer.Org(ctx, p, orgID); err != nil {
		return Org{}, err
	}
	return s.loadOrg(ctx, orgID)
}

type UpdateOrgInput struct {
	Name                        *string
	SchemaSnapshotsEnabled      *bool
	AllowConnectionSecretReveal *bool
}

// Update changes organization settings. Turning schema snapshots off purges
// the stored schema cache of every connection in the organization.
func (s *Service) Update(ctx context.Context, p access.Principal, orgID int64, in UpdateOrgInput) (Org, error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return Org{}, err
	}

	fields := map[string]string{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		in.Name = &name
		if name == "" {
			addField(fields, "name", "Name must not be empty.")
		}
	}
	if in.Name == nil && in.SchemaSnapshotsEnabled == nil && in.AllowConnectionSecretReveal == nil {
		addField(fields, "request", "At least one setting is required.")
	}
	if len(fields) != 0 {
		return Org{}, &ValidationError{Fields: fields}
	}

	current, err := s.loadOrg(ctx, orgID)
	if err != nil {
		return Org{}, err
	}
	if err := s.store.UpdateOrgSettings(ctx, orgID, in.Name, in.SchemaSnapshotsEnabled, in.AllowConnectionSecretReveal); err != nil {
		return Org{}, err
	}
	if current.SchemaSnapshotsEnabled && in.SchemaSnapshotsEnabled != nil && !*in.SchemaSnapshotsEnabled {
		if err := s.purgeSchemaCaches(ctx, orgID); err != nil {
			return Org{}, err
		}
	}

	updated, err := s.loadOrg(ctx, orgID)
	if err != nil {
		return Org{}, err
	}
	s.emit(ctx, p, orgID, ActionOrgUpdated, "org", orgID, nil)
	return updated, nil
}

// Delete deletes the organization and everything it owns.
func (s *Service) Delete(ctx context.Context, p access.Principal, orgID int64) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgDelete); err != nil {
		return err
	}
	if err := s.store.DeleteOrg(ctx, orgID); err != nil {
		return err
	}
	s.invalidateOrgPolicy(orgID)
	s.emit(ctx, p, orgID, ActionOrgDeleted, "org", orgID, nil)
	return nil
}

func (s *Service) loadOrg(ctx context.Context, orgID int64) (Org, error) {
	org, found, err := s.store.GetOrg(ctx, orgID)
	if err != nil {
		return Org{}, err
	}
	if !found {
		return Org{}, ErrNotFound
	}
	return org, nil
}

func (s *Service) purgeSchemaCaches(ctx context.Context, orgID int64) error {
	connections, err := s.store.ListOrgConnections(ctx, orgID)
	if err != nil {
		return err
	}
	s.mu.RLock()
	observers := append([]func(int64){}, s.purgeObservers...)
	s.mu.RUnlock()
	for _, conn := range connections {
		if err := s.store.DeleteSchemaCache(ctx, conn.ID); err != nil {
			return err
		}
		for _, fn := range observers {
			fn(conn.ID)
		}
	}
	return nil
}

// requireInstanceAdmin admits account principals the authentication chain
// flagged as instance admins.
func requireInstanceAdmin(p access.Principal) error {
	if p.Subject.Kind != access.SubjectAccount || !p.InstanceAdmin {
		return ErrForbidden
	}
	return nil
}

// requireOrg admits org members who hold permission on the organization.
func (s *Service) requireOrg(ctx context.Context, p access.Principal, orgID int64, permission string) error {
	if _, err := s.requirer.Org(ctx, p, orgID); err != nil {
		return err
	}
	if !s.requirer.Can(ctx, p, orgID, "org", "org", orgID, permission) {
		return ErrForbidden
	}
	return nil
}

func (s *Service) invalidateOrgPolicy(orgID int64) {
	if s.cache != nil {
		s.cache.InvalidateOrgPolicy(orgID)
	}
}

func (s *Service) invalidatePrincipals(orgID, accountID int64) {
	if s.cache != nil {
		s.cache.InvalidatePrincipals(orgID, accountID)
	}
}

// emit records a success event on a best-effort basis; a failed audit write
// must not undo or fail a change that already committed.
func (s *Service) emit(ctx context.Context, p access.Principal, orgID int64, action, resource string, resourceID int64, metadata map[string]string) {
	s.emitID(ctx, p, orgID, action, resource, strconv.FormatInt(resourceID, 10), metadata)
}

func (s *Service) emitID(ctx context.Context, p access.Principal, orgID int64, action, resource, resourceID string, metadata map[string]string) {
	err := audit.Emit(ctx, s.audit, audit.Event{
		OrgID:      &orgID,
		Actor:      audit.ActorFromPrincipal(p),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Outcome:    audit.OutcomeSuccess,
		Metadata:   metadata,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "organization audit failed", slog.String("action", action), slog.Int64("org_id", orgID), slog.Any("error", err))
	}
}

// addField keeps the first message reported for a field.
func addField(fields map[string]string, field, message string) {
	if _, exists := fields[field]; !exists {
		fields[field] = message
	}
}
