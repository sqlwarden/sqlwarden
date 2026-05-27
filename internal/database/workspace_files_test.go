package database

import (
	"context"
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
			second, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageKey:  "current",
				ContentHash: "second",
				SizeBytes:   6,
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			if first.ID != second.ID || second.Version != 1 {
				t.Fatalf("disabled revisions must replace current content: first=%+v second=%+v", first, second)
			}

			third, err := db.SaveWorkspaceFileContent(ctx, file.ID, ownerID, WorkspaceFileContent{
				StorageKey:  "versions/2",
				ContentHash: "third",
				SizeBytes:   5,
			}, true)
			if err != nil {
				t.Fatal(err)
			}
			if third.ID == second.ID || third.Version != 2 {
				t.Fatalf("versioned content must create next revision: %+v", third)
			}
		})
	}
}

func TestIsEffectiveWorkspaceMemberIncludesTeams(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, "membership-"+driver, "Membership")
			if err != nil {
				t.Fatal(err)
			}
			memberID := testUsers["bob"].id
			if err := db.AddOrgMember(ctx, org.ID, memberID); err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Workspace", "")
			if err != nil {
				t.Fatal(err)
			}
			team, err := db.InsertTeam(ctx, org.ID, "developers", "Developers")
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AddTeamMember(ctx, team.ID, memberID); err != nil {
				t.Fatal(err)
			}
			if err := db.AddWorkspaceTeam(ctx, ws.ID, team.ID, nil); err != nil {
				t.Fatal(err)
			}
			member, err := db.IsEffectiveWorkspaceMember(ctx, org.ID, ws.ID, memberID)
			if err != nil {
				t.Fatal(err)
			}
			if !member {
				t.Fatal("expected team-derived workspace membership")
			}
		})
	}
}
