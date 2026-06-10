package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/filestore"
)

func orgPrivateFilesURL(orgSlug string, workspaceID int64) string {
	return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/files/private", orgSlug, workspaceID)
}

func orgSharedFilesURL(orgSlug string, workspaceID int64) string {
	return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/files/shared", orgSlug, workspaceID)
}

func mePrivateFilesURL(workspaceID int64) string {
	return fmt.Sprintf("/api/v1/me/workspaces/%d/files/private", workspaceID)
}

func meSharedFilesURL(workspaceID int64) string {
	return fmt.Sprintf("/api/v1/me/workspaces/%d/files/shared", workspaceID)
}

func workspaceStorageSegment(ws database.Workspace) string {
	return strconv.FormatInt(ws.ID, 10) + "-" + slugify(ws.Name)
}

func newAuthContentRequest(t *testing.T, method, url, content, token, etag string) *http.Request {
	t.Helper()
	req := newTestRequest(t, method, url, nil)
	req.Body = io.NopCloser(strings.NewReader(content))
	req.ContentLength = int64(len(content))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Bearer "+token)
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	return req
}

func decodeWorkspaceFile(t *testing.T, res testResponse) database.WorkspaceFile {
	t.Helper()
	var file database.WorkspaceFile
	if err := json.Unmarshal(res.BodyBytes, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func decodeWorkspaceFileBrowser(t *testing.T, res testResponse) files.BrowserResult {
	t.Helper()
	var result files.BrowserResult
	if err := json.Unmarshal(res.BodyBytes, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func decodeWorkspaceFilesResponse(t *testing.T, res testResponse) []database.WorkspaceFile {
	t.Helper()
	var result workspaceFilesResponse
	if err := json.Unmarshal(res.BodyBytes, &result); err != nil {
		t.Fatal(err)
	}
	return result.Files
}

func addWorkspaceMemberForFiles(t *testing.T, app *application, org database.Organization, workspace database.Workspace, email string) (database.Account, string) {
	t.Helper()
	member, tok := seedAccountWithToken(t, app, email, email)
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddWorkspaceMember(context.Background(), workspace.ID, member.ID, nil); err != nil {
		t.Fatal(err)
	}
	return member, tok
}

func TestWorkspacePrivateFilesAreOwnerOnlyAndRequireMembership(t *testing.T) {
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	member, memberTok := addWorkspaceMemberForFiles(t, app, org, ws, uniqueEmail(t, "private-member"))
	_, otherTok := addWorkspaceMemberForFiles(t, app, org, ws, uniqueEmail(t, "private-other"))

	create := send(t, newAuthRequest(t, http.MethodPost, orgPrivateFilesURL(org.Slug, ws.ID), map[string]any{
		"name": "query.sql",
	}, memberTok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)
	assert.Equal(t, *file.OwnerAccountID, member.ID)

	put := send(t, newAuthContentRequest(t, http.MethodPut, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10)+"/content", "select 1", memberTok, ""), app.routes())
	assert.Equal(t, put.StatusCode, http.StatusOK)

	ownList := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, ws.ID), nil, memberTok), app.routes())
	assert.Equal(t, ownList.StatusCode, http.StatusOK)
	ownFiles := decodeWorkspaceFilesResponse(t, ownList)
	assert.Equal(t, len(ownFiles), 1)

	otherRead := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, otherTok), app.routes())
	assert.Equal(t, otherRead.StatusCode, http.StatusNotFound)
	otherPatch := send(t, newAuthRequest(t, http.MethodPatch, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), map[string]any{"name": "stolen.sql"}, otherTok), app.routes())
	assert.Equal(t, otherPatch.StatusCode, http.StatusNotFound)
	otherDelete := send(t, newAuthRequest(t, http.MethodDelete, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, otherTok), app.routes())
	assert.Equal(t, otherDelete.StatusCode, http.StatusNotFound)
	adminRead := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, adminRead.StatusCode, http.StatusNotFound)

	if err := app.db.RemoveWorkspaceMember(context.Background(), ws.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	revoked := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, ws.ID), nil, memberTok), app.routes())
	assert.Equal(t, revoked.StatusCode, http.StatusForbidden)
	revokedFile := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, memberTok), app.routes())
	assert.Equal(t, revokedFile.StatusCode, http.StatusNotFound)
}

func TestWorkspaceSharedFilesRequireSharedFilePermission(t *testing.T) {
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	_, memberTok := addWorkspaceMemberForFiles(t, app, org, ws, uniqueEmail(t, "shared-member"))
	adminTok := wsJoinAs(t, app, org.Slug, strconv.FormatInt(ws.ID, 10), "Workspace Admin", uniqueEmail(t, "shared-admin"), ownerTok)

	create := send(t, newAuthRequest(t, http.MethodPost, orgSharedFilesURL(org.Slug, ws.ID), map[string]any{
		"name": "team.sql",
	}, ownerTok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)

	denied := send(t, newAuthRequest(t, http.MethodGet, orgSharedFilesURL(org.Slug, ws.ID), nil, memberTok), app.routes())
	assert.Equal(t, denied.StatusCode, http.StatusForbidden)

	allowed := send(t, newAuthRequest(t, http.MethodGet, orgSharedFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, adminTok), app.routes())
	assert.Equal(t, allowed.StatusCode, http.StatusOK)
}

func TestWorkspaceFileBrowserAndRecentEndpoints(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	filesURL := orgPrivateFilesURL(org.Slug, ws.ID)
	folder := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "queries", "object_type": "folder",
	}, tok), app.routes()))
	file := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "scratch.sql", "parent_id": folder.ID, "media_type": "text/plain", "file_kind": "query",
	}, tok), app.routes()))
	write := send(t, newAuthContentRequest(t, http.MethodPut, filesURL+"/"+strconv.FormatInt(file.ID, 10)+"/content", "select 1", tok, ""), app.routes())
	assert.Equal(t, write.StatusCode, http.StatusOK)

	root := send(t, newAuthRequest(t, http.MethodGet, filesURL+"/browser", nil, tok), app.routes())
	assert.Equal(t, root.StatusCode, http.StatusOK)
	rootBrowser := decodeWorkspaceFileBrowser(t, root)
	if rootBrowser.File != nil || len(rootBrowser.Path) != 0 || len(rootBrowser.Children) != 1 || rootBrowser.Children[0].ID != folder.ID {
		t.Fatalf("root browser = %+v, want root folder child", rootBrowser)
	}

	folderRes := send(t, newAuthRequest(t, http.MethodGet, filesURL+"/browser?file_id="+strconv.FormatInt(folder.ID, 10), nil, tok), app.routes())
	assert.Equal(t, folderRes.StatusCode, http.StatusOK)
	folderBrowser := decodeWorkspaceFileBrowser(t, folderRes)
	if folderBrowser.File == nil || folderBrowser.File.ID != folder.ID || len(folderBrowser.Path) != 1 || folderBrowser.Path[0].Name != "queries" {
		t.Fatalf("folder browser = %+v", folderBrowser)
	}
	if len(folderBrowser.Children) != 1 || folderBrowser.Children[0].ID != file.ID || folderBrowser.Children[0].ContentHash == "" || folderBrowser.Children[0].ContentVersion != 1 {
		t.Fatalf("folder children = %+v, want enriched file child", folderBrowser.Children)
	}

	fileRes := send(t, newAuthRequest(t, http.MethodGet, filesURL+"/browser?file_id="+strconv.FormatInt(file.ID, 10), nil, tok), app.routes())
	assert.Equal(t, fileRes.StatusCode, http.StatusOK)
	fileBrowser := decodeWorkspaceFileBrowser(t, fileRes)
	if fileBrowser.File == nil || fileBrowser.File.ID != file.ID || len(fileBrowser.Path) != 2 || len(fileBrowser.Children) != 0 {
		t.Fatalf("file browser = %+v", fileBrowser)
	}

	recentRes := send(t, newAuthRequest(t, http.MethodGet, filesURL+"/recent?limit=5", nil, tok), app.routes())
	assert.Equal(t, recentRes.StatusCode, http.StatusOK)
	recent := decodeWorkspaceFilesResponse(t, recentRes)
	if len(recent) != 1 || recent[0].ID != file.ID || recent[0].ContentHash == "" {
		t.Fatalf("recent files = %+v, want enriched scratch.sql", recent)
	}

	invalid := send(t, newAuthRequest(t, http.MethodGet, filesURL+"/recent?limit=0", nil, tok), app.routes())
	assert.Equal(t, invalid.StatusCode, http.StatusUnprocessableEntity)
}

func TestWorkspacePrivateFilesAllowTeamMembersAndRejectCrossWorkspaceRoute(t *testing.T) {
	app, org, wsA, ownerTok := setupWorkspaceOwner(t)
	var creatorID int64
	if err := app.db.NewSelect().TableExpr("workspace_members").ColumnExpr("account_id").Where("workspace_id = ?", wsA.ID).Limit(1).Scan(context.Background(), &creatorID); err != nil {
		t.Fatal(err)
	}
	creator, found, err := app.db.GetAccount(context.Background(), creatorID)
	if err != nil || !found {
		t.Fatalf("get workspace creator: found=%v err=%v", found, err)
	}
	wsB := seedWorkspaceForAccount(t, app, org, creator, "Other", "")
	otherOrg := seedOrganizationForAccount(t, app, creator, "Other Organization")
	wsOtherOrg := seedWorkspaceForAccount(t, app, otherOrg, creator, "Other Organization Workspace", "")

	member, memberTok := seedAccountWithToken(t, app, uniqueEmail(t, "team-private-member"), "Team Member")
	if err := app.db.AddOrgMember(context.Background(), org.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	team, err := app.db.InsertTeam(context.Background(), org.ID, "file-team", "File Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddTeamMember(context.Background(), team.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	if err := app.db.AddWorkspaceTeam(context.Background(), wsA.ID, team.ID, &creatorID); err != nil {
		t.Fatal(err)
	}

	create := send(t, newAuthRequest(t, http.MethodPost, orgPrivateFilesURL(org.Slug, wsA.ID), map[string]any{"name": "team-private.sql"}, memberTok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)

	crossWorkspace := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, wsB.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, crossWorkspace.StatusCode, http.StatusNotFound)
	crossOrg := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(otherOrg.Slug, wsOtherOrg.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, crossOrg.StatusCode, http.StatusNotFound)
}

func TestPersonalWorkspaceFilesArePrivateAndFeatureGated(t *testing.T) {
	app := newTestApp(t)
	account, tok := seedAccountWithToken(t, app, uniqueEmail(t, "personal-files"), "Personal Files")
	_, otherTok := seedAccountWithToken(t, app, uniqueEmail(t, "personal-files-other"), "Other")
	ws, err := app.db.InsertWorkspace(context.Background(), nil, "space", account.ID, "Experiments", "")
	if err != nil {
		t.Fatal(err)
	}

	shared := send(t, newAuthRequest(t, http.MethodPost, meSharedFilesURL(ws.ID), map[string]any{"name": "bad.sql"}, tok), app.routes())
	assert.Equal(t, shared.StatusCode, http.StatusNotFound)

	create := send(t, newAuthRequest(t, http.MethodPost, mePrivateFilesURL(ws.ID), map[string]any{"name": "mine.sql"}, tok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)
	assert.Equal(t, file.Visibility, database.FileVisibilityPrivate)
	assert.Equal(t, *file.OwnerAccountID, account.ID)

	rename := send(t, newAuthRequest(t, http.MethodPatch, mePrivateFilesURL(ws.ID)+"/"+strconv.FormatInt(file.ID, 10), map[string]any{"name": "renamed.sql"}, tok), app.routes())
	assert.Equal(t, rename.StatusCode, http.StatusOK)
	remove := send(t, newAuthRequest(t, http.MethodDelete, mePrivateFilesURL(ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, tok), app.routes())
	assert.Equal(t, remove.StatusCode, http.StatusNoContent)
	deleted := send(t, newAuthRequest(t, http.MethodGet, mePrivateFilesURL(ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, tok), app.routes())
	assert.Equal(t, deleted.StatusCode, http.StatusNotFound)

	crossOwner := send(t, newAuthRequest(t, http.MethodGet, mePrivateFilesURL(ws.ID), nil, otherTok), app.routes())
	assert.Equal(t, crossOwner.StatusCode, http.StatusNotFound)

	app.config.PersonalSpacesEnabled = false
	gated := send(t, newAuthRequest(t, http.MethodGet, mePrivateFilesURL(ws.ID), nil, tok), app.routes())
	assert.Equal(t, gated.StatusCode, http.StatusNotFound)
}

func TestWorkspaceFileContentRejectsStaleExternalWrite(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	create := send(t, newAuthRequest(t, http.MethodPost, orgPrivateFilesURL(org.Slug, ws.ID), map[string]any{"name": "conflict.sql"}, tok), app.routes())
	file := decodeWorkspaceFile(t, create)
	contentURL := orgPrivateFilesURL(org.Slug, ws.ID) + "/" + strconv.FormatInt(file.ID, 10) + "/content"

	initial := send(t, newAuthContentRequest(t, http.MethodPut, contentURL, "select 1", tok, ""), app.routes())
	assert.Equal(t, initial.StatusCode, http.StatusOK)
	initialETag := initial.Header.Get("ETag")
	if initialETag == "" {
		t.Fatal("expected initial ETag")
	}

	file, found, err := app.db.GetWorkspaceFile(context.Background(), file.ID)
	if err != nil || !found {
		t.Fatalf("refresh stored file: found=%v err=%v", found, err)
	}
	content, found, err := app.db.CurrentWorkspaceFileContent(context.Background(), file)
	if err != nil || !found {
		t.Fatalf("read stored content: found=%v err=%v", found, err)
	}
	if !strings.HasPrefix(content.StorageKey, "objects/") || strings.Contains(content.StorageKey, file.Name) {
		t.Fatalf("object-store key must be opaque, got %q", content.StorageKey)
	}
	store, err := app.fileStores.Store(context.Background(), content.StorageBackendID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), content.StorageKey, strings.NewReader("external edit")); err != nil {
		t.Fatal(err)
	}

	stale := send(t, newAuthContentRequest(t, http.MethodPut, contentURL, "overwrite", tok, initialETag), app.routes())
	assert.Equal(t, stale.StatusCode, http.StatusConflict)
	read := send(t, newAuthRequest(t, http.MethodGet, contentURL, nil, tok), app.routes())
	assert.Equal(t, read.StatusCode, http.StatusOK)
	assert.Equal(t, string(read.BodyBytes), "external edit")
}

func TestWorkspaceDirectoryWritesVisibleFilePath(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	root := t.TempDir()
	store, err := filestore.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	app.fileStores = &fileStoreRegistry{activeBackendID: database.DefaultFileStorageBackendID, stores: map[string]filestore.Store{database.DefaultFileStorageBackendID: store}}
	app.config.Files.StorageMode = FilesStorageModeFile

	create := send(t, newAuthRequest(t, http.MethodPost, orgPrivateFilesURL(org.Slug, ws.ID), map[string]any{"name": "visible.sql"}, tok), app.routes())
	file := decodeWorkspaceFile(t, create)
	contentURL := orgPrivateFilesURL(org.Slug, ws.ID) + "/" + strconv.FormatInt(file.ID, 10) + "/content"
	write := send(t, newAuthContentRequest(t, http.MethodPut, contentURL, "select visible", tok, ""), app.routes())
	assert.Equal(t, write.StatusCode, http.StatusOK)

	expected := filepath.Join(root, "organizations", org.Slug, "workspaces", workspaceStorageSegment(ws), "my-files", strconv.FormatInt(file.CreatedBy, 10), "visible.sql")
	bytesOnDisk, err := os.ReadFile(expected)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytesOnDisk, []byte("select visible")) {
		t.Fatalf("visible file content = %q", bytesOnDisk)
	}
}

func TestObjectStoreVersionsTextFilesButReplacesBinaryFilesByDefault(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	app.config.Files.Revisions.DefaultPolicy = FilesRevisionPolicyVersioned

	saveTwice := func(name string) database.WorkspaceFileContent {
		create := send(t, newAuthRequest(t, http.MethodPost, orgPrivateFilesURL(org.Slug, ws.ID), map[string]any{"name": name}, tok), app.routes())
		assert.Equal(t, create.StatusCode, http.StatusCreated)
		file := decodeWorkspaceFile(t, create)
		contentURL := orgPrivateFilesURL(org.Slug, ws.ID) + "/" + strconv.FormatInt(file.ID, 10) + "/content"
		first := send(t, newAuthContentRequest(t, http.MethodPut, contentURL, "first", tok, ""), app.routes())
		assert.Equal(t, first.StatusCode, http.StatusOK)
		second := send(t, newAuthContentRequest(t, http.MethodPut, contentURL, "second", tok, first.Header.Get("ETag")), app.routes())
		assert.Equal(t, second.StatusCode, http.StatusOK)
		file, found, err := app.db.GetWorkspaceFile(context.Background(), file.ID)
		if err != nil || !found {
			t.Fatalf("get saved file: found=%v err=%v", found, err)
		}
		content, found, err := app.db.CurrentWorkspaceFileContent(context.Background(), file)
		if err != nil || !found {
			t.Fatalf("get saved content: found=%v err=%v", found, err)
		}
		return content
	}

	textContent := saveTwice("report.sql")
	if textContent.Version != 2 {
		t.Fatalf("SQL files should be versioned in object storage, got version %d", textContent.Version)
	}
	binaryContent := saveTwice("snapshot.db")
	if binaryContent.Version != 1 {
		t.Fatalf("binary files should be replaced by default, got version %d", binaryContent.Version)
	}
}

func TestWorkspaceFilesMoveTraverseAndDeleteSubtree(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	filesURL := orgPrivateFilesURL(org.Slug, ws.ID)
	folderA := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "first", "object_type": "folder",
	}, tok), app.routes()))
	folderB := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "second", "object_type": "folder",
	}, tok), app.routes()))
	file := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "before.sql", "parent_id": folderA.ID,
	}, tok), app.routes()))
	fileURL := filesURL + "/" + strconv.FormatInt(file.ID, 10)
	write := send(t, newAuthContentRequest(t, http.MethodPut, fileURL+"/content", "select moved", tok, ""), app.routes())
	assert.Equal(t, write.StatusCode, http.StatusOK)

	move := send(t, newAuthRequest(t, http.MethodPatch, fileURL, map[string]any{
		"name": "after.sql", "parent_id": folderB.ID,
	}, tok), app.routes())
	assert.Equal(t, move.StatusCode, http.StatusOK)
	oldChildren := send(t, newAuthRequest(t, http.MethodGet, filesURL+"?parent_id="+strconv.FormatInt(folderA.ID, 10), nil, tok), app.routes())
	oldFiles := decodeWorkspaceFilesResponse(t, oldChildren)
	assert.Equal(t, len(oldFiles), 0)
	newChildren := send(t, newAuthRequest(t, http.MethodGet, filesURL+"?parent_id="+strconv.FormatInt(folderB.ID, 10), nil, tok), app.routes())
	newFiles := decodeWorkspaceFilesResponse(t, newChildren)
	assert.Equal(t, len(newFiles), 1)
	assert.Equal(t, newFiles[0].Name, "after.sql")

	toRoot := send(t, newAuthRequest(t, http.MethodPatch, fileURL, map[string]any{"parent_id": nil}, tok), app.routes())
	assert.Equal(t, toRoot.StatusCode, http.StatusOK)
	rootFiles := send(t, newAuthRequest(t, http.MethodGet, filesURL, nil, tok), app.routes())
	rootItems := decodeWorkspaceFilesResponse(t, rootFiles)
	if len(rootItems) != 3 {
		t.Fatalf("root after moving file to root = %+v", rootItems)
	}
	backToFolder := send(t, newAuthRequest(t, http.MethodPatch, fileURL, map[string]any{"parent_id": folderB.ID}, tok), app.routes())
	assert.Equal(t, backToFolder.StatusCode, http.StatusOK)

	remove := send(t, newAuthRequest(t, http.MethodDelete, filesURL+"/"+strconv.FormatInt(folderB.ID, 10), nil, tok), app.routes())
	assert.Equal(t, remove.StatusCode, http.StatusNoContent)
	childAfterDelete := send(t, newAuthRequest(t, http.MethodGet, fileURL, nil, tok), app.routes())
	assert.Equal(t, childAfterDelete.StatusCode, http.StatusNotFound)
}

func TestSharedWorkspaceFileMutationsRequirePermissions(t *testing.T) {
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	_, memberTok := addWorkspaceMemberForFiles(t, app, org, ws, uniqueEmail(t, "shared-mutation-member"))
	adminTok := wsJoinAs(t, app, org.Slug, strconv.FormatInt(ws.ID, 10), "Workspace Admin", uniqueEmail(t, "shared-mutation-admin"), ownerTok)
	filesURL := orgSharedFilesURL(org.Slug, ws.ID)
	file := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "shared.sql",
	}, ownerTok), app.routes()))
	fileURL := filesURL + "/" + strconv.FormatInt(file.ID, 10)

	wrongTreeRead := send(t, newAuthRequest(t, http.MethodGet, orgPrivateFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, adminTok), app.routes())
	assert.Equal(t, wrongTreeRead.StatusCode, http.StatusNotFound)

	deniedPatch := send(t, newAuthRequest(t, http.MethodPatch, fileURL, map[string]any{"name": "denied.sql"}, memberTok), app.routes())
	assert.Equal(t, deniedPatch.StatusCode, http.StatusForbidden)
	deniedDelete := send(t, newAuthRequest(t, http.MethodDelete, fileURL, nil, memberTok), app.routes())
	assert.Equal(t, deniedDelete.StatusCode, http.StatusForbidden)

	rename := send(t, newAuthRequest(t, http.MethodPatch, fileURL, map[string]any{"name": "allowed.sql"}, adminTok), app.routes())
	assert.Equal(t, rename.StatusCode, http.StatusOK)
	remove := send(t, newAuthRequest(t, http.MethodDelete, fileURL, nil, adminTok), app.routes())
	assert.Equal(t, remove.StatusCode, http.StatusNoContent)
}

func TestWorkspaceFolderCannotMoveIntoDescendant(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	filesURL := orgPrivateFilesURL(org.Slug, ws.ID)
	parent := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "parent", "object_type": "folder",
	}, tok), app.routes()))
	child := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "child", "object_type": "folder", "parent_id": parent.ID,
	}, tok), app.routes()))
	move := send(t, newAuthRequest(t, http.MethodPatch, filesURL+"/"+strconv.FormatInt(parent.ID, 10), map[string]any{
		"parent_id": child.ID,
	}, tok), app.routes())
	assert.Equal(t, move.StatusCode, http.StatusUnprocessableEntity)
}

func TestWorkspaceFileRenameRejectsSiblingNameConflict(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	filesURL := orgPrivateFilesURL(org.Slug, ws.ID)
	send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{"name": "exists.sql"}, tok), app.routes())
	file := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{"name": "rename.sql"}, tok), app.routes()))
	conflict := send(t, newAuthRequest(t, http.MethodPatch, filesURL+"/"+strconv.FormatInt(file.ID, 10), map[string]any{"name": "exists.sql"}, tok), app.routes())
	assert.Equal(t, conflict.StatusCode, http.StatusUnprocessableEntity)
}

func TestWorkspaceDirectoryMovesTrackedDescendantsAndRejectsExternalDestination(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	root := t.TempDir()
	store, err := filestore.NewFilesystem(root)
	if err != nil {
		t.Fatal(err)
	}
	app.fileStores = &fileStoreRegistry{activeBackendID: database.DefaultFileStorageBackendID, stores: map[string]filestore.Store{database.DefaultFileStorageBackendID: store}}
	app.config.Files.StorageMode = FilesStorageModeFile
	filesURL := orgPrivateFilesURL(org.Slug, ws.ID)
	folder := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "queries", "object_type": "folder",
	}, tok), app.routes()))
	file := decodeWorkspaceFile(t, send(t, newAuthRequest(t, http.MethodPost, filesURL, map[string]any{
		"name": "report.sql", "parent_id": folder.ID,
	}, tok), app.routes()))
	contentURL := filesURL + "/" + strconv.FormatInt(file.ID, 10) + "/content"
	assert.Equal(t, send(t, newAuthContentRequest(t, http.MethodPut, contentURL, "select report", tok, ""), app.routes()).StatusCode, http.StatusOK)

	folderURL := filesURL + "/" + strconv.FormatInt(folder.ID, 10)
	assert.Equal(t, send(t, newAuthRequest(t, http.MethodPatch, folderURL, map[string]any{"name": "renamed"}, tok), app.routes()).StatusCode, http.StatusOK)
	base := filepath.Join(root, "organizations", org.Slug, "workspaces", workspaceStorageSegment(ws), "my-files", strconv.FormatInt(file.CreatedBy, 10))
	if _, err := os.Stat(filepath.Join(base, "queries", "report.sql")); !os.IsNotExist(err) {
		t.Fatalf("old tracked path should be removed, got error %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "queries")); !os.IsNotExist(err) {
		t.Fatalf("old empty folder should be pruned, got error %v", err)
	}
	bytesOnDisk, err := os.ReadFile(filepath.Join(base, "renamed", "report.sql"))
	if err != nil || string(bytesOnDisk) != "select report" {
		t.Fatalf("relocated content = %q err=%v", bytesOnDisk, err)
	}
	if err := os.MkdirAll(filepath.Join(base, "external"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "external", "report.sql"), []byte("external"), 0o640); err != nil {
		t.Fatal(err)
	}
	conflict := send(t, newAuthRequest(t, http.MethodPatch, folderURL, map[string]any{"name": "external"}, tok), app.routes())
	assert.Equal(t, conflict.StatusCode, http.StatusConflict)
}
