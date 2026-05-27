package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
)

const (
	FileVisibilityPrivate = "private"
	FileVisibilityShared  = "shared"
	FileObjectTypeFile    = "file"
	FileObjectTypeFolder  = "folder"
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
			return fmt.Errorf("invalid parent folder")
		}
	}
	now := time.Now()
	file.CreatedAt = now
	file.UpdatedAt = now
	_, err := db.NewInsert().Model(file).Returning("id").Exec(ctx)
	return err
}

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
			return nil, fmt.Errorf("invalid parent folder")
		}
		names = append([]string{parent.Name}, names...)
		parentID = parent.ParentID
	}
	return names, nil
}

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

func (db *DB) CurrentWorkspaceFileContent(ctx context.Context, file WorkspaceFile) (WorkspaceFileContent, bool, error) {
	if file.CurrentContentID == nil {
		return WorkspaceFileContent{}, false, nil
	}
	return db.GetWorkspaceFileContent(ctx, *file.CurrentContentID)
}

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

func (db *DB) IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var exists bool
	err := db.NewRaw(`
SELECT EXISTS (
    SELECT 1 FROM workspace_members wm
    JOIN workspaces w ON w.id = wm.workspace_id AND w.owner_type = 'org' AND w.owner_id = ?
    JOIN org_members om ON om.org_id = ? AND om.account_id = wm.account_id
    WHERE wm.workspace_id = ? AND wm.account_id = ?
    UNION
    SELECT 1 FROM workspace_teams wt
    JOIN workspaces w ON w.id = wt.workspace_id AND w.owner_type = 'org' AND w.owner_id = ?
    JOIN teams t ON t.id = wt.team_id AND t.org_id = ?
    JOIN team_members tm ON tm.team_id = t.id AND tm.account_id = ?
    JOIN org_members om ON om.org_id = ? AND om.account_id = tm.account_id
    WHERE wt.workspace_id = ?
)`, orgID, orgID, workspaceID, accountID, orgID, orgID, accountID, orgID, workspaceID).Scan(ctx, &exists)
	return exists, err
}

func sameNullableID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
