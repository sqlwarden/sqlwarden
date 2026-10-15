package identity

import (
	"context"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// ListInstanceSessions lists any account's auth sessions to an instance
// admin.
func (s *Service) ListInstanceSessions(ctx context.Context, p access.Principal, accountID int64, page Page) (response.Paginated[database.AuthSession], error) {
	if err := requireInstanceAdmin(p); err != nil {
		return response.Paginated[database.AuthSession]{}, err
	}
	if _, err := s.loadAccount(ctx, accountID); err != nil {
		return response.Paginated[database.AuthSession]{}, err
	}
	return s.listSessions(ctx, accountID, page)
}

// RevokeInstanceSession lets an instance admin revoke one auth session of an
// account. A session of another account is reported as not found.
func (s *Service) RevokeInstanceSession(ctx context.Context, p access.Principal, accountID int64, sessionID string) error {
	if err := requireInstanceAdmin(p); err != nil {
		return err
	}
	return s.revokeSession(ctx, accountID, sessionID, p.Subject.ID, "instance_admin_revoked")
}

// RevokeInstanceSessions lets an instance admin revoke every auth session of
// an account.
func (s *Service) RevokeInstanceSessions(ctx context.Context, p access.Principal, accountID int64) error {
	if err := requireInstanceAdmin(p); err != nil {
		return err
	}
	if _, err := s.loadAccount(ctx, accountID); err != nil {
		return err
	}
	revokedBy := p.Subject.ID
	if err := s.store.RevokeAuthSessionsForAccount(ctx, accountID, &revokedBy, "instance_admin_revoked_all"); err != nil {
		return err
	}
	s.publishAccountRevoked(ctx, accountID)
	return nil
}

// ListOrgMemberAccessSessions lists a member's access sessions for the
// organization to a caller holding org:write on it.
func (s *Service) ListOrgMemberAccessSessions(ctx context.Context, p access.Principal, orgID, accountID int64, page Page) (response.Paginated[database.OrgAccessSession], error) {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return response.Paginated[database.OrgAccessSession]{}, err
	}
	member, err := s.store.IsOrgMember(ctx, orgID, accountID)
	if err != nil {
		return response.Paginated[database.OrgAccessSession]{}, err
	}
	if !member {
		return response.Paginated[database.OrgAccessSession]{}, ErrNotFound
	}
	return s.store.ListOrgAccessSessionsPage(ctx, orgID, accountID, page.Page, page.PageSize)
}

// RevokeOrgMemberAccessSession revokes one of a member's access sessions for
// the organization. It requires org:write; a session of another account or
// organization is reported as not found.
func (s *Service) RevokeOrgMemberAccessSession(ctx context.Context, p access.Principal, orgID, accountID int64, sessionID string) error {
	if err := s.requireOrg(ctx, p, orgID, access.PermOrgWrite); err != nil {
		return err
	}
	session, found, err := s.store.GetOrgAccessSessionByID(ctx, strings.TrimSpace(sessionID), orgID, accountID)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	revokedBy := p.Subject.ID
	if err := s.store.RevokeOrgAccessSession(ctx, session.ID, orgID, &revokedBy, "org_admin_revoked"); err != nil {
		return err
	}
	s.publishAccountRevoked(ctx, accountID)
	return nil
}

func requireInstanceAdmin(p access.Principal) error {
	if err := requireAccount(p); err != nil {
		return err
	}
	if !p.InstanceAdmin {
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
