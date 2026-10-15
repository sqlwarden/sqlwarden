package query

import (
	"context"
	"io"
	"log/slog"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/response"
)

const (
	modeBackend = "backend"

	messageHistoryDisabled   = "Query history is not enabled for this connection."
	messageFavoritesDisabled = "Query favorites are not enabled for this workspace."
)

// Store is the persistence the query use cases need.
type Store interface {
	InsertQueryHistoryEntry(ctx context.Context, entry database.QueryHistoryEntry, retentionCount int) (database.QueryHistoryEntry, error)
	ListQueryHistory(ctx context.Context, connectionID, accountID int64, search string, page, pageSize int) (response.Paginated[database.QueryHistoryEntry], error)
	ListQueryHistoryForWorkspace(ctx context.Context, workspaceID, accountID int64, connectionID *int64, search string, page, pageSize int) (response.Paginated[database.QueryHistoryEntry], error)
	DeleteQueryHistoryEntry(ctx context.Context, id, connectionID, accountID int64) (bool, error)
	ClearQueryHistoryForConnection(ctx context.Context, connectionID, accountID int64) error

	ListQueryFavorites(ctx context.Context, workspaceID, accountID int64, search string) ([]database.QueryFavorite, error)
	CreateQueryFavorite(ctx context.Context, fav database.QueryFavorite) (database.QueryFavorite, error)
	UpdateQueryFavorite(ctx context.Context, id, workspaceID, accountID int64, name, sqlText string) (database.QueryFavorite, bool, error)
	DeleteQueryFavorite(ctx context.Context, id, workspaceID, accountID int64) (bool, error)
}

// Policy is the effective runtime configuration for a workspace.
type Policy struct {
	HistoryMode      string
	HistoryRetention int
	FavoritesMode    string
	MaxRows          int
	MaxBytes         int64
	CursorPageSize   int
}

// Settings resolves the effective Policy for a workspace.
type Settings interface {
	ForWorkspace(ctx context.Context, ws access.Workspace) (Policy, error)
}

// Runtime is the subset of the execution runtime the query use cases drive.
type Runtime interface {
	execution.Sessions
	execution.Queries
	execution.Transactions
}

type Deps struct {
	Store     Store
	Settings  Settings
	Requirer  *access.Requirer
	Runtime   Runtime
	Analyzers Analyzers
	Audit     audit.Writer
	Logger    *slog.Logger
}

type Service struct {
	deps Deps
}

func New(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if deps.Analyzers == nil {
		deps.Analyzers = EngineAnalyzers{}
	}
	return &Service{deps: deps}
}

// ListQuery is the search and pagination input for list operations.
type ListQuery struct {
	Search   string
	Page     int
	PageSize int
}

func (s *Service) accountID(p access.Principal) int64 { return p.Subject.ID }
