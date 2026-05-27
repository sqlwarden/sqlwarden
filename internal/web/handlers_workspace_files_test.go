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
	"github.com/sqlwarden/internal/filestore"
)

func orgFilesURL(orgSlug string, workspaceID int64) string {
	return fmt.Sprintf("/api/v1/orgs/%s/workspaces/%d/files", orgSlug, workspaceID)
}

func meFilesURL(workspaceID int64) string {
	return fmt.Sprintf("/api/v1/me/workspaces/%d/files", workspaceID)
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

	create := send(t, newAuthRequest(t, http.MethodPost, orgFilesURL(org.Slug, ws.ID), map[string]any{
		"name":       "query.sql",
		"visibility": "private",
	}, memberTok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)
	assert.Equal(t, *file.OwnerAccountID, member.ID)

	put := send(t, newAuthContentRequest(t, http.MethodPut, orgFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10)+"/content", "select 1", memberTok, ""), app.routes())
	assert.Equal(t, put.StatusCode, http.StatusOK)

	ownList := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"?visibility=private", nil, memberTok), app.routes())
	assert.Equal(t, ownList.StatusCode, http.StatusOK)
	var ownFiles []database.WorkspaceFile
	decodeJSONResponse(t, ownList.BodyBytes, &ownFiles)
	assert.Equal(t, len(ownFiles), 1)

	otherRead := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, otherTok), app.routes())
	assert.Equal(t, otherRead.StatusCode, http.StatusNotFound)
	adminRead := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, adminRead.StatusCode, http.StatusNotFound)

	if err := app.db.RemoveWorkspaceMember(context.Background(), ws.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	revoked := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"?visibility=private", nil, memberTok), app.routes())
	assert.Equal(t, revoked.StatusCode, http.StatusForbidden)
	revokedFile := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, memberTok), app.routes())
	assert.Equal(t, revokedFile.StatusCode, http.StatusNotFound)
}

func TestWorkspaceSharedFilesRequireSharedFilePermission(t *testing.T) {
	app, org, ws, ownerTok := setupWorkspaceOwner(t)
	_, memberTok := addWorkspaceMemberForFiles(t, app, org, ws, uniqueEmail(t, "shared-member"))
	adminTok := wsJoinAs(t, app, org.Slug, strconv.FormatInt(ws.ID, 10), "Workspace Admin", uniqueEmail(t, "shared-admin"), ownerTok)

	create := send(t, newAuthRequest(t, http.MethodPost, orgFilesURL(org.Slug, ws.ID), map[string]any{
		"name":       "team.sql",
		"visibility": "shared",
	}, ownerTok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)

	denied := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"?visibility=shared", nil, memberTok), app.routes())
	assert.Equal(t, denied.StatusCode, http.StatusForbidden)

	allowed := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, ws.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, adminTok), app.routes())
	assert.Equal(t, allowed.StatusCode, http.StatusOK)
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

	create := send(t, newAuthRequest(t, http.MethodPost, orgFilesURL(org.Slug, wsA.ID), map[string]any{"name": "team-private.sql"}, memberTok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)

	crossWorkspace := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(org.Slug, wsB.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, ownerTok), app.routes())
	assert.Equal(t, crossWorkspace.StatusCode, http.StatusNotFound)
	crossOrg := send(t, newAuthRequest(t, http.MethodGet, orgFilesURL(otherOrg.Slug, wsOtherOrg.ID)+"/"+strconv.FormatInt(file.ID, 10), nil, ownerTok), app.routes())
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

	shared := send(t, newAuthRequest(t, http.MethodPost, meFilesURL(ws.ID), map[string]any{
		"name":       "bad.sql",
		"visibility": "shared",
	}, tok), app.routes())
	assert.Equal(t, shared.StatusCode, http.StatusUnprocessableEntity)

	create := send(t, newAuthRequest(t, http.MethodPost, meFilesURL(ws.ID), map[string]any{"name": "mine.sql"}, tok), app.routes())
	assert.Equal(t, create.StatusCode, http.StatusCreated)
	file := decodeWorkspaceFile(t, create)
	assert.Equal(t, file.Visibility, database.FileVisibilityPrivate)
	assert.Equal(t, *file.OwnerAccountID, account.ID)

	crossOwner := send(t, newAuthRequest(t, http.MethodGet, meFilesURL(ws.ID), nil, otherTok), app.routes())
	assert.Equal(t, crossOwner.StatusCode, http.StatusNotFound)

	app.config.PersonalSpacesEnabled = false
	gated := send(t, newAuthRequest(t, http.MethodGet, meFilesURL(ws.ID), nil, tok), app.routes())
	assert.Equal(t, gated.StatusCode, http.StatusNotFound)
}

func TestWorkspaceFileContentRejectsStaleExternalWrite(t *testing.T) {
	app, org, ws, tok := setupWorkspaceOwner(t)
	create := send(t, newAuthRequest(t, http.MethodPost, orgFilesURL(org.Slug, ws.ID), map[string]any{"name": "conflict.sql"}, tok), app.routes())
	file := decodeWorkspaceFile(t, create)
	contentURL := orgFilesURL(org.Slug, ws.ID) + "/" + strconv.FormatInt(file.ID, 10) + "/content"

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
	if _, err := app.fileStore.Put(context.Background(), content.StorageKey, strings.NewReader("external edit")); err != nil {
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
	app.fileStore = store
	app.config.Files.StorageModel = FilesStorageModelWorkspaceDirectory

	create := send(t, newAuthRequest(t, http.MethodPost, orgFilesURL(org.Slug, ws.ID), map[string]any{"name": "visible.sql"}, tok), app.routes())
	file := decodeWorkspaceFile(t, create)
	contentURL := orgFilesURL(org.Slug, ws.ID) + "/" + strconv.FormatInt(file.ID, 10) + "/content"
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
		create := send(t, newAuthRequest(t, http.MethodPost, orgFilesURL(org.Slug, ws.ID), map[string]any{"name": name}, tok), app.routes())
		assert.Equal(t, create.StatusCode, http.StatusCreated)
		file := decodeWorkspaceFile(t, create)
		contentURL := orgFilesURL(org.Slug, ws.ID) + "/" + strconv.FormatInt(file.ID, 10) + "/content"
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
