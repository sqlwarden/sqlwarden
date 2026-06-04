package files

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/filestore"
)

type serviceFixture struct {
	ctx      context.Context
	db       *database.DB
	store    *filestore.Filesystem
	enforcer *fakeEnforcer
	service  *Service
	org      database.Organization
	ws       database.Workspace
	owner    database.Account
	member   database.Account
	other    database.Account
}

type fakeEnforcer struct {
	allowed map[string]bool
	calls   []string
}

type mutableStoreResolver struct {
	active string
	stores map[string]filestore.Store
}

func (r *mutableStoreResolver) ActiveBackendID() string {
	return r.active
}

func (r *mutableStoreResolver) Store(_ context.Context, backendID string) (filestore.Store, error) {
	store, ok := r.stores[backendID]
	if !ok {
		return nil, ErrStorageBackendUnavailable
	}
	return store, nil
}

func (e *fakeEnforcer) Can(_ context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	e.calls = append(e.calls, permission+"@"+ownerType+":"+resourceType+":"+strconv.FormatInt(resourceID, 10))
	return e.allowed[permission]
}

func newServiceFixture(t *testing.T, config Config) serviceFixture {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "sqlwarden-files-test.db")
	db, err := database.New("sqlite", dbPath, slog.New(slog.NewTextHandler(io.Discard, nil)), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.MigrateUp(); err != nil {
		t.Fatal(err)
	}

	store, err := filestore.NewFilesystem(filepath.Join(t.TempDir(), "objects"))
	if err != nil {
		t.Fatal(err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	owner := insertTestAccount(t, db, "files-owner-"+suffix+"@example.com", "Files Owner")
	member := insertTestAccount(t, db, "files-member-"+suffix+"@example.com", "Files Member")
	other := insertTestAccount(t, db, "files-other-"+suffix+"@example.com", "Files Other")
	org, err := db.InsertOrg(ctx, "files-"+suffix, "Files Org")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []database.Account{owner, member, other} {
		if err := db.AddOrgMember(ctx, org.ID, account.ID); err != nil {
			t.Fatal(err)
		}
	}
	ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main Workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range []database.Account{owner, member} {
		if err := db.AddWorkspaceMember(ctx, ws.ID, account.ID, nil); err != nil {
			t.Fatal(err)
		}
	}

	enforcer := &fakeEnforcer{allowed: map[string]bool{}}
	service := New(db, store, enforcer, config, &sync.Map{})
	return serviceFixture{ctx: ctx, db: db, store: store, enforcer: enforcer, service: service, org: org, ws: ws, owner: owner, member: member, other: other}
}

func insertTestAccount(t *testing.T, db *database.DB, email, name string) database.Account {
	t.Helper()
	account, err := db.InsertAccount(context.Background(), email, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func (f serviceFixture) privateScope(account database.Account) Scope {
	return Scope{
		AccountID:  account.ID,
		OrgID:      f.org.ID,
		OrgSlug:    f.org.Slug,
		Workspace:  f.ws,
		Visibility: database.FileVisibilityPrivate,
	}
}

func (f serviceFixture) sharedScope(account database.Account) Scope {
	return Scope{
		AccountID:  account.ID,
		OrgID:      f.org.ID,
		OrgSlug:    f.org.Slug,
		Workspace:  f.ws,
		Visibility: database.FileVisibilityShared,
	}
}

func TestValidName(t *testing.T) {
	for _, name := range []string{"query.sql", "query with spaces.sql", "data-01.csv"} {
		if !ValidName(name) {
			t.Fatalf("expected %q to be valid", name)
		}
	}
	for _, name := range []string{"", "   ", ".", "..", "nested/path.sql", `nested\path.sql`} {
		if ValidName(name) {
			t.Fatalf("expected %q to be invalid", name)
		}
	}
}

func TestPrivateFilesAreOwnerOnlyAndRequireEffectiveWorkspaceMembership(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})

	file, err := f.service.Create(f.ctx, f.privateScope(f.member), CreateInput{Name: "scratch.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if file.OwnerAccountID == nil || *file.OwnerAccountID != f.member.ID {
		t.Fatalf("private file owner = %v, want %d", file.OwnerAccountID, f.member.ID)
	}

	if _, err := f.service.Get(f.ctx, f.privateScope(f.owner), file.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner reading another private file error = %v, want %v", err, ErrNotFound)
	}
	if _, err := f.service.List(f.ctx, f.privateScope(f.other), nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-workspace member list error = %v, want %v", err, ErrForbidden)
	}

	teamMember := insertTestAccount(t, f.db, "team-member-"+strconv.FormatInt(time.Now().UnixNano(), 36)+"@example.com", "Team Member")
	if err := f.db.AddOrgMember(f.ctx, f.org.ID, teamMember.ID); err != nil {
		t.Fatal(err)
	}
	team, err := f.db.InsertTeam(f.ctx, f.org.ID, "team-members", "Team Members")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddTeamMember(f.ctx, team.ID, teamMember.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.db.AddWorkspaceTeam(f.ctx, f.ws.ID, team.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Create(f.ctx, f.privateScope(teamMember), CreateInput{Name: "team-scratch.sql"}); err != nil {
		t.Fatalf("team-derived workspace member create private file: %v", err)
	}
}

func TestSharedFilesRequireSharedFilePermissions(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.sharedScope(f.member)

	if _, err := f.service.Create(f.ctx, scope, CreateInput{Name: "team.sql"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("create without permission error = %v, want %v", err, ErrForbidden)
	}

	f.enforcer.allowed[access.PermWsFileCreate] = true
	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "team.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if file.OwnerAccountID != nil {
		t.Fatalf("shared file owner = %v, want nil", file.OwnerAccountID)
	}
	if _, err := f.service.Get(f.ctx, scope, file.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("read without permission error = %v, want %v", err, ErrForbidden)
	}

	f.enforcer.allowed[access.PermWsFileRead] = true
	if _, err := f.service.Get(f.ctx, scope, file.ID); err != nil {
		t.Fatalf("read with permission: %v", err)
	}
}

func TestCreateRejectsInvalidObjectsAndParents(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	if _, err := f.service.Create(f.ctx, scope, CreateInput{Name: "bad/folder.sql"}); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("invalid name error = %v, want %v", err, ErrInvalidName)
	}
	if _, err := f.service.Create(f.ctx, scope, CreateInput{Name: "folder", ObjectType: database.FileObjectTypeFolder, MediaType: "text/plain"}); !errors.Is(err, ErrInvalidObjectType) {
		t.Fatalf("folder media error = %v, want %v", err, ErrInvalidObjectType)
	}

	parentFile, err := f.service.Create(f.ctx, scope, CreateInput{Name: "not-a-folder.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Create(f.ctx, scope, CreateInput{Name: "child.sql", ParentID: &parentFile.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("file parent error = %v, want %v", err, ErrNotFound)
	}

	sharedParent, err := f.service.Create(f.ctx, scope, CreateInput{Name: "private-folder", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	f.enforcer.allowed[access.PermWsFileCreate] = true
	if _, err := f.service.Create(f.ctx, f.sharedScope(f.member), CreateInput{Name: "wrong-tree.sql", ParentID: &sharedParent.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong visibility parent error = %v, want %v", err, ErrNotFound)
	}
}

func TestBrowserReturnsRootBreadcrumbChildrenAndFileMetadata(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	folder, err := f.service.Create(f.ctx, scope, CreateInput{Name: "queries", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql", ParentID: &folder.ID, MediaType: "text/plain", FileKind: "query"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}

	root, err := f.service.Browser(f.ctx, scope, nil)
	if err != nil {
		t.Fatal(err)
	}
	if root.File != nil || len(root.Path) != 0 || len(root.Children) != 1 || root.Children[0].ID != folder.ID {
		t.Fatalf("root browser result = %+v, want root with folder child", root)
	}

	folderView, err := f.service.Browser(f.ctx, scope, &folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	if folderView.File == nil || folderView.File.ID != folder.ID || len(folderView.Path) != 1 || folderView.Path[0].Name != "queries" {
		t.Fatalf("folder browser path = %+v file=%+v", folderView.Path, folderView.File)
	}
	if len(folderView.Children) != 1 || folderView.Children[0].ID != file.ID || folderView.Children[0].ContentHash != content.ContentHash {
		t.Fatalf("folder children = %+v, want enriched file child", folderView.Children)
	}

	fileView, err := f.service.Browser(f.ctx, scope, &file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fileView.File == nil || fileView.File.ID != file.ID || fileView.File.ContentVersion != 1 || fileView.File.SizeBytes != int64(len("select 1")) {
		t.Fatalf("file browser metadata = %+v", fileView.File)
	}
	if len(fileView.Path) != 2 || fileView.Path[0].Name != "queries" || fileView.Path[1].Name != "scratch.sql" {
		t.Fatalf("file path = %+v, want queries/scratch.sql", fileView.Path)
	}
	if len(fileView.Children) != 0 {
		t.Fatalf("file children = %+v, want none", fileView.Children)
	}
}

func TestRecentReturnsOnlyAuthorizedOpenableFiles(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	if _, err := f.service.Create(f.ctx, scope, CreateInput{Name: "folder", ObjectType: database.FileObjectTypeFolder}); err != nil {
		t.Fatal(err)
	}
	oldFile, err := f.service.Create(f.ctx, scope, CreateInput{Name: "old.sql"})
	if err != nil {
		t.Fatal(err)
	}
	oldContent, err := f.service.WriteContent(f.ctx, scope, oldFile.ID, "", strings.NewReader("select old"))
	if err != nil {
		t.Fatal(err)
	}
	newFile, err := f.service.Create(f.ctx, scope, CreateInput{Name: "new.sql"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, newFile.ID, "", strings.NewReader("select new")); err != nil {
		t.Fatal(err)
	}

	recent, err := f.service.Recent(f.ctx, scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].ID != newFile.ID || recent[1].ID != oldFile.ID {
		t.Fatalf("recent files = %+v, want new then old", recent)
	}
	if recent[1].ContentHash != oldContent.ContentHash || recent[1].ContentVersion != 1 {
		t.Fatalf("recent file content metadata = %+v", recent[1])
	}

	otherRecent, err := f.service.Recent(f.ctx, f.privateScope(f.owner), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherRecent) != 0 {
		t.Fatalf("recent files for another private owner = %+v, want none", otherRecent)
	}
}

func TestObjectStoreVersionsTextFilesAndKeepsKeysStableOnRename(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyVersioned, RevisionKeepLatest: 10})
	scope := f.privateScope(f.member)

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql", MediaType: "text/plain", FileKind: "query"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || !strings.HasSuffix(first.StorageKey, "/versions/1") {
		t.Fatalf("first version/key = %d/%q, want version 1 under versions/1", first.Version, first.StorageKey)
	}

	renamed := "renamed.sql"
	if _, err := f.service.Update(f.ctx, scope, file.ID, UpdateInput{Name: &renamed}); err != nil {
		t.Fatal(err)
	}
	second, err := f.service.WriteContent(f.ctx, scope, file.ID, first.ContentHash, strings.NewReader("select 2"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || !strings.HasSuffix(second.StorageKey, "/versions/2") {
		t.Fatalf("second version/key = %d/%q, want version 2 under versions/2", second.Version, second.StorageKey)
	}
	if !strings.Contains(second.StorageKey, "/"+strconv.FormatInt(file.ID, 10)+"/") || strings.Contains(second.StorageKey, "renamed.sql") {
		t.Fatalf("object-store key %q should be opaque and file-id based", second.StorageKey)
	}

	result, err := f.service.ReadContent(f.ctx, scope, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Reader.Close()
	body, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "select 2" {
		t.Fatalf("content = %q, want select 2", string(body))
	}
}

func TestObjectStoreRevisionRetentionPrunesOldMetadataAndObjects(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyVersioned, RevisionKeepLatest: 2})
	scope := f.privateScope(f.member)

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql", MediaType: "text/plain", FileKind: "query"})
	if err != nil {
		t.Fatal(err)
	}

	var versions []database.WorkspaceFileContent
	expectedHash := ""
	for i := 1; i <= 5; i++ {
		content, err := f.service.WriteContent(f.ctx, scope, file.ID, expectedHash, strings.NewReader("select "+strconv.Itoa(i)))
		if err != nil {
			t.Fatal(err)
		}
		versions = append(versions, content)
		expectedHash = content.ContentHash
	}

	contents, err := f.db.ListWorkspaceFileSubtreeContents(f.ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 5 {
		t.Fatalf("content rows before reaper = %d, want all 5 still present", len(contents))
	}
	queued, err := f.db.ListWorkspaceFileContentDeletionBatch(f.ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 2 {
		t.Fatalf("queued deletions = %d, want 2", len(queued))
	}

	processed, err := f.service.ReapContentDeletionsOnce(f.ctx, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 2 {
		t.Fatalf("processed deletions = %d, want 2", processed)
	}

	contents, err = f.db.ListWorkspaceFileSubtreeContents(f.ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 3 {
		t.Fatalf("retained content rows after reaper = %d, want current plus 2 older", len(contents))
	}
	retained := map[int64]bool{}
	for _, content := range contents {
		retained[content.ID] = true
	}
	for _, version := range []database.WorkspaceFileContent{versions[2], versions[3], versions[4]} {
		if !retained[version.ID] {
			t.Fatalf("expected version %d to be retained; retained=%+v", version.Version, contents)
		}
		if reader, _, err := f.store.Get(f.ctx, version.StorageKey); err != nil {
			t.Fatalf("retained version %d object missing: %v", version.Version, err)
		} else {
			reader.Close()
		}
	}
	for _, version := range []database.WorkspaceFileContent{versions[0], versions[1]} {
		if retained[version.ID] {
			t.Fatalf("expected version %d metadata to be pruned", version.Version)
		}
		if _, _, err := f.store.Get(f.ctx, version.StorageKey); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("pruned version %d get err = %v, want %v", version.Version, err, os.ErrNotExist)
		}
	}
}

func TestObjectStoreRevisionRetentionKeepLatestZeroKeepsOnlyCurrent(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyVersioned, RevisionKeepLatest: 0})
	scope := f.privateScope(f.member)

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql", MediaType: "text/plain", FileKind: "query"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.WriteContent(f.ctx, scope, file.ID, first.ContentHash, strings.NewReader("select 2"))
	if err != nil {
		t.Fatal(err)
	}

	contents, err := f.db.ListWorkspaceFileSubtreeContents(f.ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 2 {
		t.Fatalf("content rows before reaper = %+v, want both versions", contents)
	}
	processed, err := f.service.ReapContentDeletionsOnce(f.ctx, 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("processed deletions = %d, want 1", processed)
	}
	contents, err = f.db.ListWorkspaceFileSubtreeContents(f.ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 1 || contents[0].ID != second.ID {
		t.Fatalf("retained content rows after reaper = %+v, want only current", contents)
	}
	if _, _, err := f.store.Get(f.ctx, first.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old object get err = %v, want %v", err, os.ErrNotExist)
	}
}

func TestContentReadsUseSavedBackendAndWritesUseActiveBackend(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, ActiveStorageBackendID: "local", RevisionPolicy: RevisionPolicyVersioned, RevisionKeepLatest: 10})
	archive, err := filestore.NewFilesystem(filepath.Join(t.TempDir(), "archive"))
	if err != nil {
		t.Fatal(err)
	}
	resolver := &mutableStoreResolver{
		active: "local",
		stores: map[string]filestore.Store{
			"local":   f.store,
			"archive": archive,
		},
	}
	f.service = NewWithStoreResolver(f.db, resolver, f.enforcer, Config{StorageMode: StorageModeObject, ActiveStorageBackendID: "local", RevisionPolicy: RevisionPolicyVersioned, RevisionKeepLatest: 10}, &sync.Map{})
	scope := f.privateScope(f.member)

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("local copy"))
	if err != nil {
		t.Fatal(err)
	}
	if first.StorageBackendID != "local" {
		t.Fatalf("first storage backend = %q, want local", first.StorageBackendID)
	}

	resolver.active = "archive"
	readFirst, err := f.service.ReadContent(f.ctx, scope, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	firstBody, err := io.ReadAll(readFirst.Reader)
	readFirst.Reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBody) != "local copy" {
		t.Fatalf("read after active backend switch = %q, want local copy", string(firstBody))
	}

	second, err := f.service.WriteContent(f.ctx, scope, file.ID, first.ContentHash, strings.NewReader("archive copy"))
	if err != nil {
		t.Fatal(err)
	}
	if second.StorageBackendID != "archive" {
		t.Fatalf("second storage backend = %q, want archive", second.StorageBackendID)
	}
	if _, _, err := f.store.Get(f.ctx, second.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("second content should not be in local store, err=%v", err)
	}
	if reader, _, err := archive.Get(f.ctx, second.StorageKey); err != nil {
		t.Fatalf("second content missing from archive: %v", err)
	} else {
		reader.Close()
	}
}

func TestReadContentFailsWhenSavedBackendIsUnavailable(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, ActiveStorageBackendID: "local", RevisionPolicy: RevisionPolicyDisabled})
	resolver := &mutableStoreResolver{active: "local", stores: map[string]filestore.Store{"local": f.store}}
	f.service = NewWithStoreResolver(f.db, resolver, f.enforcer, Config{StorageMode: StorageModeObject, ActiveStorageBackendID: "local", RevisionPolicy: RevisionPolicyDisabled}, &sync.Map{})
	scope := f.privateScope(f.member)

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("content"))
	if err != nil {
		t.Fatal(err)
	}
	delete(resolver.stores, content.StorageBackendID)

	if _, err := f.service.ReadContent(f.ctx, scope, file.ID); !errors.Is(err, ErrStorageBackendUnavailable) {
		t.Fatalf("read missing backend error = %v, want %v", err, ErrStorageBackendUnavailable)
	}
}

func TestDeleteFailsBeforeTombstoneWhenSavedBackendIsUnavailable(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, ActiveStorageBackendID: "local", RevisionPolicy: RevisionPolicyDisabled})
	resolver := &mutableStoreResolver{active: "local", stores: map[string]filestore.Store{"local": f.store}}
	f.service = NewWithStoreResolver(f.db, resolver, f.enforcer, Config{StorageMode: StorageModeObject, ActiveStorageBackendID: "local", RevisionPolicy: RevisionPolicyDisabled}, &sync.Map{})
	scope := f.privateScope(f.member)

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("content"))
	if err != nil {
		t.Fatal(err)
	}
	delete(resolver.stores, content.StorageBackendID)

	if err := f.service.Delete(f.ctx, scope, file.ID); !errors.Is(err, ErrStorageBackendUnavailable) {
		t.Fatalf("delete missing backend error = %v, want %v", err, ErrStorageBackendUnavailable)
	}
	if _, found, err := f.db.GetWorkspaceFile(f.ctx, file.ID); err != nil || !found {
		t.Fatalf("file should not be tombstoned when backend is missing: found=%v err=%v", found, err)
	}
}

func TestWriteContentRequiresCurrentHashAndRejectsFolders(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	folder, err := f.service.Create(f.ctx, scope, CreateInput{Name: "folder", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, folder.ID, "", strings.NewReader("nope")); !errors.Is(err, ErrFolderContent) {
		t.Fatalf("folder write error = %v, want %v", err, ErrFolderContent)
	}

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "scratch.sql"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 2")); !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("missing if-match error = %v, want %v", err, ErrPreconditionRequired)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, file.ID, "wrong", strings.NewReader("select 2")); !errors.Is(err, ErrStaleContent) {
		t.Fatalf("stale if-match error = %v, want %v", err, ErrStaleContent)
	}
	if _, err := f.service.WriteContent(f.ctx, scope, file.ID, first.ContentHash, strings.NewReader("select 2")); err != nil {
		t.Fatalf("matching if-match write: %v", err)
	}
}

func TestWorkspaceDirectoryRelocatesContentAndPrunesOldPath(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeFile, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	folder, err := f.service.Create(f.ctx, scope, CreateInput{Name: "reports", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "daily.sql", ParentID: &folder.ID})
	if err != nil {
		t.Fatal(err)
	}
	first, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select count(*)"))
	if err != nil {
		t.Fatal(err)
	}
	if reader, _, err := f.store.Get(f.ctx, first.StorageKey); err != nil {
		t.Fatalf("stored visible file missing before rename: %v", err)
	} else {
		reader.Close()
	}

	renamedFolder := "renamed-reports"
	if _, err := f.service.Update(f.ctx, scope, folder.ID, UpdateInput{Name: &renamedFolder}); err != nil {
		t.Fatal(err)
	}
	currentFile, found, err := f.db.GetWorkspaceFile(f.ctx, file.ID)
	if err != nil || !found {
		t.Fatalf("get child after rename found=%v err=%v", found, err)
	}
	current, found, err := f.db.CurrentWorkspaceFileContent(f.ctx, currentFile)
	if err != nil || !found {
		t.Fatalf("current content after rename found=%v err=%v", found, err)
	}
	wantSuffix := "organizations/" + f.org.Slug + "/workspaces/" + workspaceStorageSegment(f.ws) + "/my-files/" + strconv.FormatInt(f.member.ID, 10) + "/renamed-reports/daily.sql"
	if current.StorageKey != wantSuffix {
		t.Fatalf("storage key = %q, want %q", current.StorageKey, wantSuffix)
	}
	if _, _, err := f.store.Get(f.ctx, first.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old key get error = %v, want %v", err, os.ErrNotExist)
	}
	if _, err := os.Stat(filepath.Join(f.store.Root(), filepath.FromSlash("organizations/"+f.org.Slug+"/workspaces/"+workspaceStorageSegment(f.ws)+"/my-files/"+strconv.FormatInt(f.member.ID, 10)+"/reports"))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old directory stat error = %v, want %v", err, os.ErrNotExist)
	}
}

func TestWorkspaceDirectoryRejectsMoveCyclesAndStorageCollisions(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeFile, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	parent, err := f.service.Create(f.ctx, scope, CreateInput{Name: "parent", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	child, err := f.service.Create(f.ctx, scope, CreateInput{Name: "child", ParentID: &parent.ID, ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Update(f.ctx, scope, parent.ID, UpdateInput{ParentID: &child.ID, ParentIDSet: true}); !errors.Is(err, ErrMoveCycle) {
		t.Fatalf("move cycle error = %v, want %v", err, ErrMoveCycle)
	}
	if _, err := f.service.Update(f.ctx, scope, parent.ID, UpdateInput{}); !errors.Is(err, ErrMissingUpdate) {
		t.Fatalf("missing update error = %v, want %v", err, ErrMissingUpdate)
	}

	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "collision.sql"})
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}
	newName := "collision-renamed.sql"
	collisionKey := strings.TrimSuffix(content.StorageKey, "collision.sql") + newName
	if _, err := f.store.Put(f.ctx, collisionKey, strings.NewReader("untracked")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Update(f.ctx, scope, file.ID, UpdateInput{Name: &newName}); !errors.Is(err, ErrStorageDestinationExists) {
		t.Fatalf("storage collision error = %v, want %v", err, ErrStorageDestinationExists)
	}
}

func TestDeleteRemovesMetadataTreeAndStoredContent(t *testing.T) {
	f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
	scope := f.privateScope(f.member)

	folder, err := f.service.Create(f.ctx, scope, CreateInput{Name: "folder", ObjectType: database.FileObjectTypeFolder})
	if err != nil {
		t.Fatal(err)
	}
	file, err := f.service.Create(f.ctx, scope, CreateInput{Name: "query.sql", ParentID: &folder.ID})
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.service.WriteContent(f.ctx, scope, file.ID, "", strings.NewReader("select 1"))
	if err != nil {
		t.Fatal(err)
	}

	if err := f.service.Delete(f.ctx, scope, folder.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Get(f.ctx, scope, file.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted child error = %v, want %v", err, ErrNotFound)
	}
	if _, _, err := f.store.Get(f.ctx, content.StorageKey); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stored content get error = %v, want %v", err, os.ErrNotExist)
	}
}
