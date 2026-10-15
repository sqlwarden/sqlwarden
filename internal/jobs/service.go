package jobs

import (
	"context"
	"strconv"
	"strings"

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

const (
	defaultPage     = 1
	defaultPageSize = 25
	maxPageSize     = 100
)

// ValidationError carries field-safe paging validation messages.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return "jobs: validation failed" }

// parsePaging parses raw paging parameters after authorization, so a caller
// without workspace access learns nothing about the shape of valid input.
// An empty value selects the default.
func parsePaging(rawPage, rawPageSize string) (int, int, error) {
	page, pageSize := defaultPage, defaultPageSize
	fields := map[string]string{}
	if raw := strings.TrimSpace(rawPage); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			fields["page"] = "Page must be a positive integer."
		} else {
			page = value
		}
	}
	if raw := strings.TrimSpace(rawPageSize); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > maxPageSize {
			fields["page_size"] = "Page size must be between 1 and 100."
		} else {
			pageSize = value
		}
	}
	if len(fields) != 0 {
		return 0, 0, &ValidationError{Fields: fields}
	}
	return page, pageSize, nil
}

func (s *Service) List(ctx context.Context, p access.Principal, orgID, wsID int64, rawPage, rawPageSize string) (response.Paginated[Record], error) {
	if _, err := s.requirer.WorkspaceMember(ctx, p, orgID, wsID); err != nil {
		return response.Paginated[Record]{}, err
	}
	page, pageSize, err := parsePaging(rawPage, rawPageSize)
	if err != nil {
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

func (s *Service) Events(ctx context.Context, p access.Principal, orgID, wsID int64, jobID, afterID, rawPageSize string) (EventPage, error) {
	if _, err := s.requirer.WorkspaceMember(ctx, p, orgID, wsID); err != nil {
		return EventPage{}, err
	}
	_, pageSize, err := parsePaging("", rawPageSize)
	if err != nil {
		return EventPage{}, err
	}
	return s.store.ListUserWorkspaceJobEvents(ctx, orgID, wsID, p.Subject.ID, jobID, afterID, pageSize)
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
