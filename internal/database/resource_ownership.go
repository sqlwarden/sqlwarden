package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

func (db *DB) workspaceHierarchyOwner(ctx context.Context, workspaceID int64) (string, int64, error) {
	return db.workspaceHierarchyOwnerWithExecutor(ctx, db.DB, workspaceID)
}

func (db *DB) workspaceHierarchyOwnerWithExecutor(ctx context.Context, exec bun.IDB, workspaceID int64) (string, int64, error) {
	var ws Workspace
	err := exec.NewSelect().Model(&ws).Where("id = ?", workspaceID).Scan(ctx)
	if err != nil {
		return "", 0, err
	}
	if ws.OwnerType == "" {
		return "", 0, errors.New("workspace owner_type is empty")
	}
	if ws.ID == 0 {
		return "", 0, fmt.Errorf("workspace %d not found", workspaceID)
	}
	return ws.OwnerType, ws.OwnerID, nil
}
