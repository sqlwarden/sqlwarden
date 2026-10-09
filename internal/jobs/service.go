package jobs

import (
	"context"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/response"
)

type Store interface {
	ListUserWorkspaceJobs(ctx context.Context, orgID, workspaceID, accountID int64, page, pageSize int) (response.Paginated[Record], error)
	GetUserWorkspaceJob(ctx context.Context, orgID, workspaceID, accountID int64, jobID string) (Record, bool, error)
	ListUserWorkspaceJobEvents(ctx context.Context, orgID, workspaceID, accountID int64, jobID, afterID string, limit int) (EventPage, error)
	RequestCancelUserWorkspaceJob(ctx context.Context, orgID, workspaceID, accountID int64, jobID string) (Record, bool, error)
}

var _ Store = (*DatabaseStore)(nil)

type Service struct {
	store    Store
	requirer *access.Requirer
}

func NewService(store Store, requirer *access.Requirer) *Service {
	return &Service{store: store, requirer: requirer}
}

func (s *Service) List(ctx context.Context, p access.Principal, orgID, wsID int64, page, pageSize int) (response.Paginated[Record], error) {
	if _, err := s.requirer.WorkspaceMember(ctx, p, orgID, wsID); err != nil {
		return response.Paginated[Record]{}, err
	}
	return s.store.ListUserWorkspaceJobs(ctx, orgID, wsID, p.Subject.ID, page, pageSize)
}

func (s *Service) Get(ctx context.Context, p access.Principal, orgID, wsID int64, jobID string) (Record, error) {
	if _, err := s.requirer.WorkspaceMember(ctx, p, orgID, wsID); err != nil {
		return Record{}, err
	}
	job, found, err := s.store.GetUserWorkspaceJob(ctx, orgID, wsID, p.Subject.ID, jobID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, ErrNotFound
	}
	return job, nil
}

func (s *Service) Events(ctx context.Context, p access.Principal, orgID, wsID int64, jobID, afterID string, limit int) (EventPage, error) {
	if _, err := s.requirer.WorkspaceMember(ctx, p, orgID, wsID); err != nil {
		return EventPage{}, err
	}
	return s.store.ListUserWorkspaceJobEvents(ctx, orgID, wsID, p.Subject.ID, jobID, afterID, limit)
}

func (s *Service) RequestCancel(ctx context.Context, p access.Principal, orgID, wsID int64, jobID string) (Record, error) {
	if _, err := s.requirer.WorkspaceMember(ctx, p, orgID, wsID); err != nil {
		return Record{}, err
	}
	job, found, err := s.store.RequestCancelUserWorkspaceJob(ctx, orgID, wsID, p.Subject.ID, jobID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, ErrNotFound
	}
	return job, nil
}
