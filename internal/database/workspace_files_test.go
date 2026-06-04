package database

import (
	"context"
	"errors"
	"testing"
	"time"
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
			ancestors, err := db.WorkspaceFileAncestors(ctx, file)
			if err != nil {
				t.Fatal(err)
			}
			if len(ancestors) != 2 || ancestors[0].ID != folder.ID || ancestors[1].ID != file.ID {
				t.Fatalf("ancestors = %+v, want folder then file", ancestors)
			}
			recent, err := db.ListRecentWorkspaceFiles(ctx, ws.ID, FileVisibilityPrivate, &ownerID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(recent) != 1 || recent[0].ID != file.ID {
				t.Fatalf("recent private files = %+v, want only query.sql", recent)
			}
			bobID := testUsers["bob"].id
			bobRecent, err := db.ListRecentWorkspaceFiles(ctx, ws.ID, FileVisibilityPrivate, &bobID, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(bobRecent) != 0 {
				t.Fatalf("recent files for another owner = %+v, want none", bobRecent)
			}
		})
	}
}

func TestWorkspaceFileContentRetentionCandidates(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "retention-"+driver, "Retention")
			if err != nil {
				t.Fatal(err)
			}
			ownerID := testUsers["alice"].id
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}
			file := WorkspaceFile{
				WorkspaceID:    ws.ID,
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
			other := WorkspaceFile{
				WorkspaceID:    ws.ID,
				Visibility:     FileVisibilityPrivate,
				OwnerAccountID: &ownerID,
				ObjectType:     FileObjectTypeFile,
				Name:           "other.sql",
				CreatedBy:      ownerID,
				UpdatedBy:      ownerID,
			}
			if err := db.InsertWorkspaceFile(ctx, &other); err != nil {
				t.Fatal(err)
			}

			var versions []WorkspaceFileContent
			for i := 1; i <= 5; i++ {
				content, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
					StorageKey:  "versions/" + string(rune('0'+i)),
					ContentHash: "hash-" + string(rune('0'+i)),
					SizeBytes:   int64(i),
				}, true)
				if err != nil {
					t.Fatal(err)
				}
				versions = append(versions, content)
			}
			if _, err := db.SaveWorkspaceFileContent(ctx, other.ID, ownerID, WorkspaceFileContent{
				StorageKey:  "versions/1",
				ContentHash: "other",
				SizeBytes:   1,
			}, true); err != nil {
				t.Fatal(err)
			}

			candidates, err := db.ListWorkspaceFileContentRetentionCandidates(ctx, file.ID, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates) != 2 || candidates[0].ID != versions[1].ID || candidates[1].ID != versions[0].ID {
				t.Fatalf("candidates = %+v, want versions 2 then 1", candidates)
			}

			deleted, err := db.DeleteWorkspaceFileContentIfNotCurrent(ctx, versions[4].ID)
			if err != nil {
				t.Fatal(err)
			}
			if deleted {
				t.Fatal("expected current content delete to be ignored")
			}
			if _, found, err := db.GetWorkspaceFileContent(ctx, versions[4].ID); err != nil || !found {
				t.Fatalf("current content deleted or unavailable: found=%v err=%v", found, err)
			}
			deleted, err = db.DeleteWorkspaceFileContentIfNotCurrent(ctx, versions[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if !deleted {
				t.Fatal("expected old content to be deleted")
			}
			if _, found, err := db.GetWorkspaceFileContent(ctx, versions[0].ID); err != nil || found {
				t.Fatalf("old content found=%v err=%v, want deleted", found, err)
			}
		})
	}
}

func TestWorkspaceFileContentDeletionQueue(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "deletion-queue-"+driver, "Deletion Queue")
			if err != nil {
				t.Fatal(err)
			}
			ownerID := testUsers["alice"].id
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}
			file := WorkspaceFile{
				WorkspaceID:    ws.ID,
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
			first, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageKey:  "versions/1",
				ContentHash: "first",
				SizeBytes:   1,
			}, true)
			if err != nil {
				t.Fatal(err)
			}
			second, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageKey:  "versions/2",
				ContentHash: "second",
				SizeBytes:   1,
			}, true)
			if err != nil {
				t.Fatal(err)
			}

			if err := db.EnqueueWorkspaceFileContentDeletions(ctx, []WorkspaceFileContent{first, first, second}); err != nil {
				t.Fatal(err)
			}
			batch, err := db.ListWorkspaceFileContentDeletionBatch(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(batch) != 2 {
				t.Fatalf("deletion batch length = %d, want 2", len(batch))
			}
			if err := db.MarkWorkspaceFileContentDeletionFailed(ctx, batch[0].ID, "failed", time.Minute); err != nil {
				t.Fatal(err)
			}
			ready, err := db.ListWorkspaceFileContentDeletionBatch(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(ready) != 1 {
				t.Fatalf("ready deletion batch length = %d, want 1 after retry delay", len(ready))
			}
			if err := db.DeleteWorkspaceFileContentDeletion(ctx, ready[0].ID); err != nil {
				t.Fatal(err)
			}
			ready, err = db.ListWorkspaceFileContentDeletionBatch(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}
			if len(ready) != 0 {
				t.Fatalf("ready deletion batch length = %d, want 0", len(ready))
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
