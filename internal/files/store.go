package files

import (
	"context"
	"time"

	"github.com/sqlwarden/internal/database"
)

// Store is the metadata persistence the file service needs. *database.DB
// satisfies it.
type Store interface {
	IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error)

	InsertWorkspaceFile(ctx context.Context, file *database.WorkspaceFile) error
	PurgeWorkspaceFile(ctx context.Context, fileID int64) error
	GetWorkspaceFile(ctx context.Context, id int64) (database.WorkspaceFile, bool, error)
	ListWorkspaceFiles(ctx context.Context, workspaceID int64, visibility string, ownerAccountID *int64, parentID *int64) ([]database.WorkspaceFile, error)
	ListRecentWorkspaceFiles(ctx context.Context, workspaceID int64, visibility string, ownerAccountID *int64, limit int) ([]database.WorkspaceFile, error)
	ListAllWorkspaceFiles(ctx context.Context, workspaceID int64, visibility string, ownerAccountID *int64) ([]database.WorkspaceFile, error)
	WorkspaceFileAncestors(ctx context.Context, file database.WorkspaceFile) ([]database.WorkspaceFile, error)
	WorkspaceFilePath(ctx context.Context, file database.WorkspaceFile) ([]string, error)
	ListWorkspaceFileSubtree(ctx context.Context, fileID int64) ([]database.WorkspaceFile, error)
	ListWorkspaceFileSubtreeContents(ctx context.Context, fileID int64) ([]database.WorkspaceFileContent, error)
	UpdateWorkspaceFileLocation(ctx context.Context, file database.WorkspaceFile, actorID int64, storageKeys map[int64]string) error
	DeleteWorkspaceFileTree(ctx context.Context, fileID, actorID int64) error

	CurrentWorkspaceFileContent(ctx context.Context, file database.WorkspaceFile) (database.WorkspaceFileContent, bool, error)
	SaveWorkspaceFileContent(ctx context.Context, fileID, actorID int64, content database.WorkspaceFileContent, versioned bool) (database.WorkspaceFileContent, error)
	ListWorkspaceFileContentRetentionCandidates(ctx context.Context, fileID int64, keepLatest int) ([]database.WorkspaceFileContent, error)
	EnqueueWorkspaceFileContentDeletions(ctx context.Context, contents []database.WorkspaceFileContent) error
	ListWorkspaceFileContentDeletionBatch(ctx context.Context, limit int) ([]database.WorkspaceFileContentDeletion, error)
	DeleteWorkspaceFileContentDeletion(ctx context.Context, deletionID int64) error
	MarkWorkspaceFileContentDeletionFailed(ctx context.Context, deletionID int64, message string, retryAfter time.Duration) error
	DeleteWorkspaceFileContentIfNotCurrent(ctx context.Context, contentID int64) (bool, error)
}
