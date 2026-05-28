package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

const (
	FileVisibilityPrivate = "private"
	FileVisibilityShared  = "shared"
	FileObjectTypeFile    = "file"
	FileObjectTypeFolder  = "folder"
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
	StorageKey           string     `bun:",notnull" json:"-"`
	ContentHash          string     `bun:",notnull" json:"content_hash"`
	SizeBytes            int64      `bun:",notnull" json:"size_bytes"`
	ExternalModifiedAt   *time.Time `bun:",nullzero" json:"-"`
	ApplicationEncrypted bool       `bun:",notnull" json:"-"`
	EncryptionKeyID      string     `bun:",nullzero" json:"-"`
	CreatedBy            int64      `bun:",notnull" json:"created_by"`
	CreatedAt            time.Time  `bun:",notnull" json:"created_at"`
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

// WorkspaceFilePath returns the visible path segments for a file/folder.
func (db *DB) WorkspaceFilePath(ctx context.Context, file WorkspaceFile) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	names := []string{file.Name}
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
		names = append([]string{parent.Name}, names...)
		parentID = parent.ParentID
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

// CurrentWorkspaceFileContent returns the current content metadata for a file.
func (db *DB) CurrentWorkspaceFileContent(ctx context.Context, file WorkspaceFile) (WorkspaceFileContent, bool, error) {
	if file.CurrentContentID == nil {
		return WorkspaceFileContent{}, false, nil
	}
	return db.GetWorkspaceFileContent(ctx, *file.CurrentContentID)
}

// SaveWorkspaceFileContent stores content metadata, either replacing current or appending a version.
func (db *DB) SaveWorkspaceFileContent(ctx context.Context, fileID, actorID int64, content WorkspaceFileContent, versioned bool) (WorkspaceFileContent, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

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
					Column("storage_key", "content_hash", "size_bytes", "external_modified_at", "application_encrypted", "encryption_key_id", "created_by", "created_at").
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
