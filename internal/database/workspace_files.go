package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

const (
	FileVisibilityPrivate       = "private"
	FileVisibilityShared        = "shared"
	FileObjectTypeFile          = "file"
	FileObjectTypeFolder        = "folder"
	DefaultFileStorageBackendID = "local"
)

var (
	ErrInvalidWorkspaceFileParent = errors.New("invalid workspace file parent")
	ErrWorkspaceFileMoveCycle     = errors.New("workspace file cannot be moved into itself or its descendant")
)

type WorkspaceFile struct {
	bun.BaseModel    `bun:"table:workspace_files"`
	ID               int64      `bun:",pk,autoincrement" json:"id"`
	WorkspaceID      int64      `bun:",notnull" json:"workspace_id"`
	ParentID         *int64     `bun:",nullzero" json:"parent_id,omitempty"`
	Visibility       string     `bun:",notnull" json:"visibility"`
	OwnerAccountID   *int64     `bun:",nullzero" json:"owner_account_id,omitempty"`
	ObjectType       string     `bun:",notnull" json:"object_type"`
	Name             string     `bun:",notnull" json:"name"`
	MediaType        string     `bun:",nullzero" json:"media_type,omitempty"`
	FileKind         string     `bun:",nullzero" json:"file_kind,omitempty"`
	CurrentContentID *int64     `bun:",nullzero" json:"current_content_id,omitempty"`
	CreatedBy        int64      `bun:",notnull" json:"created_by"`
	UpdatedBy        int64      `bun:",notnull" json:"updated_by"`
	DeletedAt        *time.Time `bun:",nullzero" json:"deleted_at,omitempty"`
	CreatedAt        time.Time  `bun:",notnull" json:"created_at"`
	UpdatedAt        time.Time  `bun:",notnull" json:"updated_at"`
	ContentHash      string     `bun:"-" json:"content_hash,omitempty"`
	ContentVersion   int        `bun:"-" json:"content_version,omitempty"`
	SizeBytes        int64      `bun:"-" json:"size_bytes,omitempty"`
}

type WorkspaceFileContent struct {
	bun.BaseModel        `bun:"table:workspace_file_contents"`
	ID                   int64      `bun:",pk,autoincrement" json:"id"`
	FileID               int64      `bun:",notnull" json:"file_id"`
	Version              int        `bun:",notnull" json:"version"`
	StorageBackendID     string     `bun:",notnull" json:"-"`
	StorageKey           string     `bun:",notnull" json:"-"`
	ContentHash          string     `bun:",notnull" json:"content_hash"`
	SizeBytes            int64      `bun:",notnull" json:"size_bytes"`
	ExternalModifiedAt   *time.Time `bun:",nullzero" json:"-"`
	ApplicationEncrypted bool       `bun:",notnull" json:"-"`
	EncryptionKeyID      string     `bun:",nullzero" json:"-"`
	CreatedBy            int64      `bun:",notnull" json:"created_by"`
	CreatedAt            time.Time  `bun:",notnull" json:"created_at"`
}

type WorkspaceFileContentDeletion struct {
	bun.BaseModel    `bun:"table:workspace_file_content_deletions"`
	ID               int64     `bun:",pk,autoincrement"`
	ContentID        int64     `bun:",notnull"`
	StorageBackendID string    `bun:",notnull"`
	StorageKey       string    `bun:",notnull"`
	Attempts         int       `bun:",notnull"`
	NextAttemptAt    time.Time `bun:",notnull"`
	LastError        string    `bun:",nullzero"`
	CreatedAt        time.Time `bun:",notnull"`
	UpdatedAt        time.Time `bun:",notnull"`
}

// ListWorkspaceFileStorageBackendIDs returns backend IDs referenced by saved
// file content. Startup uses this to fail fast when configuration no longer
// contains a backend needed to read existing bytes.
func (db *DB) ListWorkspaceFileStorageBackendIDs(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []string
	err := db.NewSelect().
		TableExpr("workspace_file_contents").
		ColumnExpr("DISTINCT storage_backend_id").
		OrderExpr("storage_backend_id ASC").
		Scan(ctx, &ids)
	return ids, err
}

// InsertWorkspaceFile creates a file or folder after validating the parent tree.
func (db *DB) InsertWorkspaceFile(ctx context.Context, file *WorkspaceFile) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if file.ParentID != nil {
		parent, found, err := db.GetWorkspaceFile(ctx, *file.ParentID)
		if err != nil {
			return err
		}
		if !found || parent.WorkspaceID != file.WorkspaceID || parent.Visibility != file.Visibility ||
			!sameNullableID(parent.OwnerAccountID, file.OwnerAccountID) || parent.ObjectType != FileObjectTypeFolder {
			return ErrInvalidWorkspaceFileParent
		}
	}
	now := time.Now()
	file.CreatedAt = now
	file.UpdatedAt = now
	_, err := db.NewInsert().Model(file).Returning("id").Exec(ctx)
	return err
}

// GetWorkspaceFile returns a non-deleted workspace file/folder by ID.
func (db *DB) GetWorkspaceFile(ctx context.Context, id int64) (WorkspaceFile, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var file WorkspaceFile
	err := db.NewSelect().Model(&file).Where("id = ? AND deleted_at IS NULL", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceFile{}, false, nil
	}
	return file, err == nil, err
}

// ListWorkspaceFiles returns direct children for a workspace file tree.
func (db *DB) ListWorkspaceFiles(ctx context.Context, workspaceID int64, visibility string, ownerAccountID *int64, parentID *int64) ([]WorkspaceFile, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	query := db.NewSelect().Model((*WorkspaceFile)(nil)).
		Where("workspace_id = ? AND visibility = ? AND deleted_at IS NULL", workspaceID, visibility)
	if ownerAccountID == nil {
		query = query.Where("owner_account_id IS NULL")
	} else {
		query = query.Where("owner_account_id = ?", *ownerAccountID)
	}
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	var files []WorkspaceFile
	if err := query.OrderExpr("object_type DESC, name ASC, id ASC").Scan(ctx, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// ListRecentWorkspaceFiles returns recently updated files from one authorized
// workspace file tree. Folders are excluded because the IDE recent list opens
// editable content, not containers.
func (db *DB) ListRecentWorkspaceFiles(ctx context.Context, workspaceID int64, visibility string, ownerAccountID *int64, limit int) ([]WorkspaceFile, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	query := db.NewSelect().Model((*WorkspaceFile)(nil)).
		Where("workspace_id = ? AND visibility = ? AND object_type = ? AND deleted_at IS NULL", workspaceID, visibility, FileObjectTypeFile)
	if ownerAccountID == nil {
		query = query.Where("owner_account_id IS NULL")
	} else {
		query = query.Where("owner_account_id = ?", *ownerAccountID)
	}
	var files []WorkspaceFile
	if err := query.OrderExpr("updated_at DESC, id DESC").Limit(limit).Scan(ctx, &files); err != nil {
		return nil, err
	}
	return files, nil
}

// WorkspaceFileAncestors returns the parent chain plus the file itself, ordered
// from root to leaf, while verifying every parent remains in the same tree.
func (db *DB) WorkspaceFileAncestors(ctx context.Context, file WorkspaceFile) ([]WorkspaceFile, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	segments := []WorkspaceFile{file}
	parentID := file.ParentID
	for parentID != nil {
		parent, found, err := db.GetWorkspaceFile(ctx, *parentID)
		if err != nil {
			return nil, err
		}
		if !found || parent.WorkspaceID != file.WorkspaceID || parent.Visibility != file.Visibility ||
			!sameNullableID(parent.OwnerAccountID, file.OwnerAccountID) || parent.ObjectType != FileObjectTypeFolder {
			return nil, ErrInvalidWorkspaceFileParent
		}
		segments = append([]WorkspaceFile{parent}, segments...)
		parentID = parent.ParentID
	}
	return segments, nil
}

// WorkspaceFilePath returns the visible path segments for a file/folder.
func (db *DB) WorkspaceFilePath(ctx context.Context, file WorkspaceFile) ([]string, error) {
	ancestors, err := db.WorkspaceFileAncestors(ctx, file)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(ancestors))
	for _, ancestor := range ancestors {
		names = append(names, ancestor.Name)
	}
	return names, nil
}

// ListWorkspaceFileSubtree returns a file/folder and all non-deleted descendants.
func (db *DB) ListWorkspaceFileSubtree(ctx context.Context, fileID int64) ([]WorkspaceFile, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var files []WorkspaceFile
	err := db.NewRaw(`
WITH RECURSIVE subtree AS (
    SELECT *
    FROM workspace_files
    WHERE id = ? AND deleted_at IS NULL
  UNION ALL
    SELECT wf.*
    FROM workspace_files wf
    JOIN subtree parent ON wf.parent_id = parent.id
    WHERE wf.deleted_at IS NULL
)
SELECT * FROM subtree`, fileID).Scan(ctx, &files)
	return files, err
}

// ListWorkspaceFileSubtreeContents returns all content rows for a file subtree.
func (db *DB) ListWorkspaceFileSubtreeContents(ctx context.Context, fileID int64) ([]WorkspaceFileContent, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var contents []WorkspaceFileContent
	err := db.NewRaw(`
WITH RECURSIVE subtree AS (
    SELECT id
    FROM workspace_files
    WHERE id = ? AND deleted_at IS NULL
  UNION ALL
    SELECT wf.id
    FROM workspace_files wf
    JOIN subtree parent ON wf.parent_id = parent.id
    WHERE wf.deleted_at IS NULL
)
SELECT content.*
FROM workspace_file_contents content
JOIN subtree file ON file.id = content.file_id`, fileID).Scan(ctx, &contents)
	return contents, err
}

// UpdateWorkspaceFileLocation renames/moves a file and updates planned storage keys atomically.
func (db *DB) UpdateWorkspaceFileLocation(ctx context.Context, file WorkspaceFile, actorID int64, storageKeys map[int64]string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := validateWorkspaceFileLocation(ctx, tx, file); err != nil {
			return err
		}
		result, err := tx.NewUpdate().Model(&file).
			Column("name", "parent_id").
			Set("updated_by = ?, updated_at = ?", actorID, time.Now()).
			Where("id = ? AND deleted_at IS NULL", file.ID).
			Exec(ctx)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows == 0 {
			return sql.ErrNoRows
		}
		for contentID, key := range storageKeys {
			if _, err := tx.NewUpdate().TableExpr("workspace_file_contents").
				Set("storage_key = ?", key).
				Where("id = ?", contentID).
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteWorkspaceFileTree soft-deletes a file/folder and all non-deleted descendants.
func (db *DB) DeleteWorkspaceFileTree(ctx context.Context, fileID, actorID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewRaw(`
WITH RECURSIVE subtree AS (
    SELECT id
    FROM workspace_files
    WHERE id = ? AND deleted_at IS NULL
  UNION ALL
    SELECT wf.id
    FROM workspace_files wf
    JOIN subtree parent ON wf.parent_id = parent.id
    WHERE wf.deleted_at IS NULL
)
UPDATE workspace_files
SET deleted_at = ?, updated_by = ?, updated_at = ?
WHERE id IN (SELECT id FROM subtree)`, fileID, time.Now(), actorID, time.Now()).Exec(ctx)
	return err
}

// GetWorkspaceFileContent returns one content metadata row by ID.
func (db *DB) GetWorkspaceFileContent(ctx context.Context, contentID int64) (WorkspaceFileContent, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var content WorkspaceFileContent
	err := db.NewSelect().Model(&content).Where("id = ?", contentID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkspaceFileContent{}, false, nil
	}
	return content, err == nil, err
}

// ListApplicationEncryptedFileContents returns every content row whose bytes
// were sealed by the application encryption key. It is used by encryption-key
// rotation to re-encrypt at-rest file content and must not be tenant-scoped.
func (db *DB) ListApplicationEncryptedFileContents(ctx context.Context) ([]WorkspaceFileContent, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var contents []WorkspaceFileContent
	err := db.NewSelect().Model(&contents).
		Where("application_encrypted = ?", true).
		OrderExpr("id ASC").
		Scan(ctx)
	return contents, err
}

// UpdateWorkspaceFileContentEncryption records a re-encrypted content row after
// rotation: the bytes were rewritten in place, so the content hash and the key
// id that sealed them change while the storage key stays the same.
func (db *DB) UpdateWorkspaceFileContentEncryption(ctx context.Context, id int64, contentHash, encryptionKeyID string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().Model((*WorkspaceFileContent)(nil)).
		Set("content_hash = ?", contentHash).
		Set("encryption_key_id = ?", encryptionKeyID).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// CurrentWorkspaceFileContent returns the current content metadata for a file.
func (db *DB) CurrentWorkspaceFileContent(ctx context.Context, file WorkspaceFile) (WorkspaceFileContent, bool, error) {
	if file.CurrentContentID == nil {
		return WorkspaceFileContent{}, false, nil
	}
	return db.GetWorkspaceFileContent(ctx, *file.CurrentContentID)
}

// ListWorkspaceFileContentRetentionCandidates returns older content rows that
// can be pruned while preserving the current row and the newest keepLatest old
// revisions for the file.
func (db *DB) ListWorkspaceFileContentRetentionCandidates(ctx context.Context, fileID int64, keepLatest int) ([]WorkspaceFileContent, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if keepLatest < 0 {
		keepLatest = 0
	}

	var currentID *int64
	err := db.NewSelect().TableExpr("workspace_files").
		ColumnExpr("current_content_id").
		Where("id = ? AND object_type = ? AND deleted_at IS NULL", fileID, FileObjectTypeFile).
		Scan(ctx, &currentID)
	if errors.Is(err, sql.ErrNoRows) || currentID == nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var contents []WorkspaceFileContent
	err = db.NewSelect().Model(&contents).
		Where("file_id = ? AND id <> ?", fileID, *currentID).
		OrderExpr("version DESC, id DESC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	if len(contents) <= keepLatest {
		return nil, nil
	}
	return contents[keepLatest:], nil
}

// EnqueueWorkspaceFileContentDeletions records stale content rows for
// asynchronous byte deletion. Existing queue rows are left untouched.
func (db *DB) EnqueueWorkspaceFileContentDeletions(ctx context.Context, contents []WorkspaceFileContent) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if len(contents) == 0 {
		return nil
	}
	now := time.Now()
	deletions := make([]WorkspaceFileContentDeletion, 0, len(contents))
	for _, content := range contents {
		deletions = append(deletions, WorkspaceFileContentDeletion{
			ContentID:        content.ID,
			StorageBackendID: content.StorageBackendID,
			StorageKey:       content.StorageKey,
			NextAttemptAt:    now,
			CreatedAt:        now,
			UpdatedAt:        now,
		})
	}
	_, err := db.NewInsert().Model(&deletions).On("CONFLICT (content_id) DO NOTHING").Exec(ctx)
	return err
}

// ListWorkspaceFileContentDeletionBatch returns due deletion queue rows.
func (db *DB) ListWorkspaceFileContentDeletionBatch(ctx context.Context, limit int) ([]WorkspaceFileContentDeletion, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if limit <= 0 {
		return nil, nil
	}
	var deletions []WorkspaceFileContentDeletion
	err := db.NewSelect().Model(&deletions).
		Where("next_attempt_at <= ?", time.Now()).
		OrderExpr("next_attempt_at ASC, id ASC").
		Limit(limit).
		Scan(ctx)
	return deletions, err
}

// DeleteWorkspaceFileContentDeletion removes one completed queue row.
func (db *DB) DeleteWorkspaceFileContentDeletion(ctx context.Context, deletionID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*WorkspaceFileContentDeletion)(nil)).Where("id = ?", deletionID).Exec(ctx)
	return err
}

// MarkWorkspaceFileContentDeletionFailed records a failed cleanup attempt and
// schedules a later retry.
func (db *DB) MarkWorkspaceFileContentDeletionFailed(ctx context.Context, deletionID int64, message string, retryAfter time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if retryAfter <= 0 {
		retryAfter = time.Minute
	}
	_, err := db.NewUpdate().Model((*WorkspaceFileContentDeletion)(nil)).
		Set("attempts = attempts + 1").
		Set("last_error = ?", message).
		Set("next_attempt_at = ?", time.Now().Add(retryAfter)).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", deletionID).
		Exec(ctx)
	return err
}

// DeleteWorkspaceFileContentIfNotCurrent deletes one content metadata row only
// when it is not the current content row for its file.
func (db *DB) DeleteWorkspaceFileContentIfNotCurrent(ctx context.Context, contentID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	result, err := db.NewRaw(`
DELETE FROM workspace_file_contents
WHERE id = ?
  AND NOT EXISTS (
    SELECT 1
    FROM workspace_files
    WHERE current_content_id = ?
  )`, contentID, contentID).Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// SaveWorkspaceFileContent stores content metadata, either replacing current or appending a version.
func (db *DB) SaveWorkspaceFileContent(ctx context.Context, fileID, actorID int64, content WorkspaceFileContent, versioned bool) (WorkspaceFileContent, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if content.StorageBackendID == "" {
		content.StorageBackendID = DefaultFileStorageBackendID
	}

	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var currentID *int64
		var nextVersion int
		err := tx.NewSelect().TableExpr("workspace_files").
			ColumnExpr("current_content_id").
			Where("id = ? AND object_type = ? AND deleted_at IS NULL", fileID, FileObjectTypeFile).
			Scan(ctx, &currentID)
		if err != nil {
			return err
		}

		if currentID != nil {
			var current WorkspaceFileContent
			if err := tx.NewSelect().Model(&current).Where("id = ?", *currentID).Scan(ctx); err != nil {
				return err
			}
			if !versioned {
				content.ID = current.ID
				content.FileID = fileID
				content.Version = current.Version
				content.CreatedBy = actorID
				content.CreatedAt = time.Now()
				_, err := tx.NewUpdate().Model(&content).
					Column("storage_backend_id", "storage_key", "content_hash", "size_bytes", "external_modified_at", "application_encrypted", "encryption_key_id", "created_by", "created_at").
					WherePK().
					Exec(ctx)
				if err != nil {
					return err
				}
				_, err = tx.NewUpdate().TableExpr("workspace_files").
					Set("updated_by = ?, updated_at = ?", actorID, time.Now()).
					Where("id = ?", fileID).
					Exec(ctx)
				return err
			}
			nextVersion = current.Version + 1
		} else {
			nextVersion = 1
		}

		content.FileID = fileID
		content.Version = nextVersion
		content.CreatedBy = actorID
		content.CreatedAt = time.Now()
		if _, err := tx.NewInsert().Model(&content).Returning("id").Exec(ctx); err != nil {
			return err
		}
		_, err = tx.NewUpdate().TableExpr("workspace_files").
			Set("current_content_id = ?, updated_by = ?, updated_at = ?", content.ID, actorID, time.Now()).
			Where("id = ?", fileID).
			Exec(ctx)
		return err
	})
	return content, err
}

func sameNullableID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validateWorkspaceFileLocation(ctx context.Context, exec bun.IDB, file WorkspaceFile) error {
	parentID := file.ParentID
	for parentID != nil {
		if *parentID == file.ID {
			return ErrWorkspaceFileMoveCycle
		}
		var parent WorkspaceFile
		err := exec.NewSelect().Model(&parent).Where("id = ? AND deleted_at IS NULL", *parentID).Scan(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidWorkspaceFileParent
		}
		if err != nil {
			return err
		}
		if parent.WorkspaceID != file.WorkspaceID || parent.Visibility != file.Visibility ||
			!sameNullableID(parent.OwnerAccountID, file.OwnerAccountID) || parent.ObjectType != FileObjectTypeFolder {
			return ErrInvalidWorkspaceFileParent
		}
		parentID = parent.ParentID
	}
	return nil
}
