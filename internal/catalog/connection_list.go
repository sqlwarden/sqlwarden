package catalog

import (
	"context"
	"sort"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/response"
)

// ListConnectionsInput filters, sorts and pages the connections of a
// workspace. RouteEnvironmentID is the environment the request was routed
// through, which must belong to the workspace; EnvironmentID filters by
// environment. Sort is "name", "driver" or "created_at"; Order "asc" sorts
// ascending and anything else descending.
type ListConnectionsInput struct {
	OrgID              int64
	WorkspaceID        int64
	RouteEnvironmentID *int64
	EnvironmentID      *int64
	Search             string
	Driver             string
	AccessMode         string
	Sort               string
	Order              string
	Page               int
	PageSize           int
}

// ListConnections returns a page of the workspace's connections the
// principal can access. A routed environment outside the workspace is
// ErrNotFound.
func (s *Service) ListConnections(ctx context.Context, p access.Principal, in ListConnectionsInput) (response.Paginated[database.Connection], error) {
	if _, err := s.requirer().Workspace(ctx, p, in.OrgID, in.WorkspaceID, ""); err != nil {
		return response.Paginated[database.Connection]{}, err
	}
	if in.RouteEnvironmentID != nil {
		if err := s.requireEnvironmentOwnership(ctx, in.WorkspaceID, *in.RouteEnvironmentID); err != nil {
			return response.Paginated[database.Connection]{}, err
		}
		in.EnvironmentID = in.RouteEnvironmentID
	}
	conns, err := s.deps.Workspaces.ListAccessibleConnections(ctx, p.Subject.ID, in.OrgID, in.WorkspaceID)
	if err != nil {
		return response.Paginated[database.Connection]{}, err
	}
	return filterConnections(conns, in), nil
}

func filterConnections(conns []database.Connection, in ListConnectionsInput) response.Paginated[database.Connection] {
	filtered := make([]database.Connection, 0, len(conns))
	search := strings.ToLower(strings.TrimSpace(in.Search))
	for _, conn := range conns {
		if search != "" && !strings.Contains(strings.ToLower(conn.Name), search) {
			continue
		}
		if in.EnvironmentID != nil && conn.EnvironmentID != *in.EnvironmentID {
			continue
		}
		if in.Driver != "" && conn.Driver != in.Driver {
			continue
		}
		if in.AccessMode != "" && conn.AccessMode != in.AccessMode {
			continue
		}
		filtered = append(filtered, conn)
	}
	sort.Slice(filtered, func(i, j int) bool {
		cmp := compareConnection(filtered[i], filtered[j], in.Sort)
		if in.Order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
	return response.PaginateItems(filtered, in.Page, in.PageSize)
}

func compareConnection(left, right database.Connection, sortBy string) int {
	switch sortBy {
	case "name":
		if left.Name != right.Name {
			return strings.Compare(left.Name, right.Name)
		}
	case "driver":
		if left.Driver != right.Driver {
			return strings.Compare(left.Driver, right.Driver)
		}
	default:
		if !left.CreatedAt.Equal(right.CreatedAt) {
			if left.CreatedAt.Before(right.CreatedAt) {
				return -1
			}
			return 1
		}
	}
	switch {
	case left.ID < right.ID:
		return -1
	case left.ID > right.ID:
		return 1
	}
	return 0
}
