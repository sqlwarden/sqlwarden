package database

import (
	"context"
	"errors"
	"testing"
)

func TestWorkspaceFilesValidateTreeAndContentPolicy(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "files-"+driver, "Files")
			if err != nil {
				t.Fatal(err)
			}
			ownerID := testUsers["alice"].id
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}

			folder := WorkspaceFile{
				WorkspaceID:    ws.ID,
				Visibility:     FileVisibilityPrivate,
				OwnerAccountID: &ownerID,
				ObjectType:     FileObjectTypeFolder,
				Name:           "queries",
				CreatedBy:      ownerID,
				UpdatedBy:      ownerID,
			}
			if err := db.InsertWorkspaceFile(ctx, &folder); err != nil {
				t.Fatal(err)
			}
			file := WorkspaceFile{
				WorkspaceID:    ws.ID,
				ParentID:       &folder.ID,
				Visibility:     FileVisibilityPrivate,
				OwnerAccountID: &ownerID,
				ObjectType:     FileObjectTypeFile,
				Name:           "query.sql",
				CreatedBy:      ownerID,
				UpdatedBy:      ownerID,
			}
			if err := db.InsertWorkspaceFile(ctx, &file); err != nil {
				t.Fatal(err)
			}

			shared := WorkspaceFile{
				WorkspaceID: ws.ID,
				ParentID:    &folder.ID,
				Visibility:  FileVisibilityShared,
				ObjectType:  FileObjectTypeFile,
				Name:        "invalid.sql",
				CreatedBy:   ownerID,
				UpdatedBy:   ownerID,
			}
			if err := db.InsertWorkspaceFile(ctx, &shared); err == nil {
				t.Fatal("expected shared file in private folder to fail")
			}

			first, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageKey:  "current",
				ContentHash: "first",
				SizeBytes:   5,
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			if first.StorageBackendID != DefaultFileStorageBackendID {
				t.Fatalf("default storage backend = %q, want %q", first.StorageBackendID, DefaultFileStorageBackendID)
			}
			second, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageBackendID: "archive",
				StorageKey:       "current",
				ContentHash:      "second",
				SizeBytes:        6,
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			if first.ID != second.ID || second.Version != 1 || second.StorageBackendID != "archive" {
				t.Fatalf("disabled revisions must replace current content: first=%+v second=%+v", first, second)
			}

			third, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageBackendID: "cold",
				StorageKey:       "versions/2",
				ContentHash:      "third",
				SizeBytes:        5,
			}, true)
			if err != nil {
				t.Fatal(err)
			}
			if third.ID == second.ID || third.Version != 2 || third.StorageBackendID != "cold" {
				t.Fatalf("versioned content must create next revision: %+v", third)
			}
			backendIDs, err := db.ListWorkspaceFileStorageBackendIDs(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(backendIDs) != 2 || backendIDs[0] != "archive" || backendIDs[1] != "cold" {
				t.Fatalf("storage backend IDs = %+v, want [archive cold]", backendIDs)
			}
		})
	}
}

func TestWorkspaceFileMoveRejectsCyclesAndDeleteTombstonesSubtree(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "file-mutations-"+driver, "File Mutations")
			if err != nil {
				t.Fatal(err)
			}
			ownerID := testUsers["alice"].id
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}
			root := WorkspaceFile{WorkspaceID: ws.ID, Visibility: FileVisibilityPrivate, OwnerAccountID: &ownerID, ObjectType: FileObjectTypeFolder, Name: "root", CreatedBy: ownerID, UpdatedBy: ownerID}
			if err := db.InsertWorkspaceFile(ctx, &root); err != nil {
				t.Fatal(err)
			}
			child := WorkspaceFile{WorkspaceID: ws.ID, ParentID: &root.ID, Visibility: FileVisibilityPrivate, OwnerAccountID: &ownerID, ObjectType: FileObjectTypeFolder, Name: "child", CreatedBy: ownerID, UpdatedBy: ownerID}
			if err := db.InsertWorkspaceFile(ctx, &child); err != nil {
				t.Fatal(err)
			}
			file := WorkspaceFile{WorkspaceID: ws.ID, ParentID: &child.ID, Visibility: FileVisibilityPrivate, OwnerAccountID: &ownerID, ObjectType: FileObjectTypeFile, Name: "query.sql", CreatedBy: ownerID, UpdatedBy: ownerID}
			if err := db.InsertWorkspaceFile(ctx, &file); err != nil {
				t.Fatal(err)
			}

			root.ParentID = &child.ID
			if err := db.UpdateWorkspaceFileLocation(ctx, root, ownerID, nil); !errors.Is(err, ErrWorkspaceFileMoveCycle) {
				t.Fatalf("move folder into descendant error = %v, want cycle error", err)
			}
			if err := db.DeleteWorkspaceFileTree(ctx, root.ID, ownerID); err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{root.ID, child.ID, file.ID} {
				if _, found, err := db.GetWorkspaceFile(ctx, id); err != nil || found {
					t.Fatalf("deleted file %d: found=%v err=%v", id, found, err)
				}
			}
		})
	}
}
