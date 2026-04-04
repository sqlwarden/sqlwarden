package database

import (
	"context"
	"errors"
	"fmt"
)

func (db *DB) workspaceHierarchyOwner(ctx context.Context, workspaceID int64) (string, int64, error) {
	ws, found, err := db.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return "", 0, err
	}
	if !found {
		return "", 0, fmt.Errorf("workspace %d not found", workspaceID)
	}
	if ws.OwnerType == "" {
		return "", 0, errors.New("workspace owner_type is empty")
	}
	return ws.OwnerType, ws.OwnerID, nil
}
