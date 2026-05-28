package files

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/filestore"
)

const (
	StorageModeFile         = "file"
	StorageModeObject       = "object"
	RevisionPolicyDisabled  = "disabled"
	RevisionPolicyVersioned = "versioned"
)

var (
	ErrForbidden                 = errors.New("workspace file access forbidden")
	ErrNotFound                  = errors.New("workspace file not found")
	ErrStorageBackendUnavailable = errors.New("workspace file storage backend is unavailable")
	ErrInvalidName               = errors.New("workspace file name is invalid")
	ErrInvalidObjectType         = errors.New("workspace file object type is invalid")
	ErrInvalidParent             = errors.New("workspace file parent is invalid")
	ErrMoveCycle                 = errors.New("workspace file cannot be moved into itself or its descendant")
	ErrMissingUpdate             = errors.New("workspace file update requires name or parent_id")
	ErrFolderContent             = errors.New("folder content cannot be read or updated")
	ErrPreconditionRequired      = errors.New("if-match is required when updating existing file content")
	ErrStaleContent              = errors.New("workspace file content is stale")
	ErrStorageDestinationExists  = errors.New("workspace file destination exists")
)

// Enforcer is the permission check used for shared workspace-file operations.
type Enforcer interface {
	Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool
}

// Config controls logical workspace-file storage behavior. The byte backend is
// still selected by the injected filestore.Store.
type Config struct {
	StorageMode            string
	ActiveStorageBackendID string
	RevisionPolicy         string
}

// StoreResolver resolves configured storage backends by their stable backend ID.
type StoreResolver interface {
	ActiveBackendID() string
	Store(ctx context.Context, backendID string) (filestore.Store, error)
}

type singleStoreResolver struct {
	backendID string
	store     filestore.Store
}

func (r singleStoreResolver) ActiveBackendID() string {
	if r.backendID == "" {
		return database.DefaultFileStorageBackendID
	}
	return r.backendID
}

func (r singleStoreResolver) Store(_ context.Context, backendID string) (filestore.Store, error) {
	if backendID == "" {
		backendID = r.ActiveBackendID()
	}
	if backendID != r.ActiveBackendID() || r.store == nil {
		return nil, ErrStorageBackendUnavailable
	}
	return r.store, nil
}

// Scope identifies the actor, workspace, organization, and file tree for one
// workspace-file operation.
type Scope struct {
	AccountID  int64
	OrgID      int64
	OrgSlug    string
	Workspace  database.Workspace
	Visibility string
}

// CreateInput is the validated domain input for creating files or folders.
type CreateInput struct {
	Name       string
	ObjectType string
	ParentID   *int64
	MediaType  string
	FileKind   string
}

// UpdateInput is the domain input for renaming or moving a file/folder.
type UpdateInput struct {
	Name        *string
	ParentID    *int64
	ParentIDSet bool
}

// ReadContentResult contains an opened content reader and its current metadata.
type ReadContentResult struct {
	File   database.WorkspaceFile
	Object filestore.StoredObject
	Reader io.ReadCloser
}

// PathSegment is one breadcrumb entry for a workspace file browser location.
type PathSegment struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ObjectType string `json:"object_type"`
}

// BrowserResult is the IDE-oriented snapshot for a file tree location. A nil
// File means the browser is at the tree root.
type BrowserResult struct {
	File     *database.WorkspaceFile  `json:"file"`
	Path     []PathSegment            `json:"path"`
	Children []database.WorkspaceFile `json:"children"`
}

// Service owns workspace-file authorization, metadata mutations, storage-key
// planning, and content IO orchestration.
type Service struct {
	db       *database.DB
	stores   StoreResolver
	enforcer Enforcer
	config   Config
	locks    *sync.Map
}

// New creates a workspace-file service over a database, byte store, and RBAC
// enforcer. locks may be shared across service instances for process-local
// serialization.
func New(db *database.DB, store filestore.Store, enforcer Enforcer, config Config, locks *sync.Map) *Service {
	return NewWithStoreResolver(db, singleStoreResolver{backendID: config.ActiveStorageBackendID, store: store}, enforcer, config, locks)
}

// NewWithStoreResolver creates a workspace-file service that can read content
// from any configured backend and write new content to the active backend.
func NewWithStoreResolver(db *database.DB, stores StoreResolver, enforcer Enforcer, config Config, locks *sync.Map) *Service {
	if locks == nil {
		locks = &sync.Map{}
	}
	if config.ActiveStorageBackendID == "" {
		config.ActiveStorageBackendID = database.DefaultFileStorageBackendID
	}
	return &Service{db: db, stores: stores, enforcer: enforcer, config: config, locks: locks}
}

// List returns direct children from the requested private or shared file tree.
func (s *Service) List(ctx context.Context, scope Scope, parentID *int64) ([]database.WorkspaceFile, error) {
	ownerID, err := s.authorizeTree(ctx, scope, access.PermWsFileRead)
	if err != nil {
		return nil, err
	}
	if parentID != nil {
		if err := s.validateParent(ctx, scope, *parentID, ownerID); err != nil {
			return nil, err
		}
	}
	files, err := s.db.ListWorkspaceFiles(ctx, scope.Workspace.ID, scope.Visibility, ownerID, parentID)
	if err != nil {
		return nil, err
	}
	if err := s.enrichFilesWithCurrentContent(ctx, files); err != nil {
		return nil, err
	}
	return files, nil
}

// Browser returns the current node, breadcrumb path, and direct children for an
// IDE file browser location. A nil fileID represents the file-tree root.
func (s *Service) Browser(ctx context.Context, scope Scope, fileID *int64) (BrowserResult, error) {
	if fileID == nil {
		children, err := s.List(ctx, scope, nil)
		if err != nil {
			return BrowserResult{}, err
		}
		return BrowserResult{Path: []PathSegment{}, Children: children}, nil
	}

	file, err := s.Get(ctx, scope, *fileID)
	if err != nil {
		return BrowserResult{}, err
	}
	path, err := s.pathSegments(ctx, file)
	if err != nil {
		return BrowserResult{}, err
	}
	children := []database.WorkspaceFile{}
	if file.ObjectType == database.FileObjectTypeFolder {
		children, err = s.List(ctx, scope, &file.ID)
		if err != nil {
			return BrowserResult{}, err
		}
	}
	return BrowserResult{File: &file, Path: path, Children: children}, nil
}

// Recent returns recently updated files from the authorized private or shared
// file tree. It excludes folders because IDE recent entries must be openable.
func (s *Service) Recent(ctx context.Context, scope Scope, limit int) ([]database.WorkspaceFile, error) {
	ownerID, err := s.authorizeTree(ctx, scope, access.PermWsFileRead)
	if err != nil {
		return nil, err
	}
	files, err := s.db.ListRecentWorkspaceFiles(ctx, scope.Workspace.ID, scope.Visibility, ownerID, limit)
	if err != nil {
		return nil, err
	}
	if err := s.enrichFilesWithCurrentContent(ctx, files); err != nil {
		return nil, err
	}
	return files, nil
}

// Create inserts a file or folder in the requested private or shared file tree.
func (s *Service) Create(ctx context.Context, scope Scope, input CreateInput) (database.WorkspaceFile, error) {
	if input.ObjectType == "" {
		input.ObjectType = database.FileObjectTypeFile
	}
	input.Name = strings.TrimSpace(input.Name)
	input.MediaType = strings.TrimSpace(input.MediaType)
	input.FileKind = strings.TrimSpace(input.FileKind)
	if !ValidName(input.Name) {
		return database.WorkspaceFile{}, ErrInvalidName
	}
	if input.ObjectType != database.FileObjectTypeFile && input.ObjectType != database.FileObjectTypeFolder {
		return database.WorkspaceFile{}, ErrInvalidObjectType
	}
	if input.ObjectType == database.FileObjectTypeFolder && (input.MediaType != "" || input.FileKind != "") {
		return database.WorkspaceFile{}, ErrInvalidObjectType
	}
	ownerID, err := s.authorizeTree(ctx, scope, access.PermWsFileCreate)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if input.ParentID != nil {
		if err := s.validateParent(ctx, scope, *input.ParentID, ownerID); err != nil {
			return database.WorkspaceFile{}, err
		}
	}
	file := database.WorkspaceFile{
		WorkspaceID:    scope.Workspace.ID,
		ParentID:       input.ParentID,
		Visibility:     scope.Visibility,
		OwnerAccountID: ownerID,
		ObjectType:     input.ObjectType,
		Name:           input.Name,
		MediaType:      input.MediaType,
		FileKind:       input.FileKind,
		CreatedBy:      scope.AccountID,
		UpdatedBy:      scope.AccountID,
	}
	if err := s.db.InsertWorkspaceFile(ctx, &file); err != nil {
		return database.WorkspaceFile{}, err
	}
	return file, nil
}

// Get returns metadata for one authorized file/folder and enriches it with
// current content metadata when the node is a file with content.
func (s *Service) Get(ctx context.Context, scope Scope, fileID int64) (database.WorkspaceFile, error) {
	file, err := s.authorizedFile(ctx, scope, access.PermWsFileRead, fileID)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if err := s.enrichFileWithCurrentContent(ctx, &file); err != nil {
		return database.WorkspaceFile{}, err
	}
	return file, nil
}

// Update renames and/or moves an authorized file/folder and keeps file-mode
// storage paths in sync when that mode is active.
func (s *Service) Update(ctx context.Context, scope Scope, fileID int64, input UpdateInput) (database.WorkspaceFile, error) {
	lock := s.workspaceLock(scope.Workspace.ID)
	lock.Lock()
	defer lock.Unlock()

	file, err := s.authorizedFile(ctx, scope, access.PermWsFileWrite, fileID)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if input.Name == nil && !input.ParentIDSet {
		return database.WorkspaceFile{}, ErrMissingUpdate
	}
	updated := file
	if input.Name != nil {
		updated.Name = strings.TrimSpace(*input.Name)
		if !ValidName(updated.Name) {
			return database.WorkspaceFile{}, ErrInvalidName
		}
	}
	if input.ParentIDSet {
		updated.ParentID = input.ParentID
	}
	ownerID := updated.OwnerAccountID
	if updated.ParentID != nil {
		if *updated.ParentID == updated.ID {
			return database.WorkspaceFile{}, ErrMoveCycle
		}
		if err := s.validateParent(ctx, scope, *updated.ParentID, ownerID); err != nil {
			return database.WorkspaceFile{}, err
		}
	}
	if updated.ObjectType == database.FileObjectTypeFolder && updated.ParentID != nil {
		subtree, err := s.db.ListWorkspaceFileSubtree(ctx, updated.ID)
		if err != nil {
			return database.WorkspaceFile{}, err
		}
		for _, descendant := range subtree {
			if descendant.ID == *updated.ParentID {
				return database.WorkspaceFile{}, ErrMoveCycle
			}
		}
	}

	planner := s.planner()
	relocations, err := planner.Relocations(ctx, s, scope, file, updated)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if err := s.stageRelocations(ctx, relocations); err != nil {
		return database.WorkspaceFile{}, err
	}
	storageKeys := make(map[int64]string, len(relocations))
	for _, relocation := range relocations {
		storageKeys[relocation.contentID] = relocation.newKey
	}
	if err := s.db.UpdateWorkspaceFileLocation(ctx, updated, scope.AccountID, storageKeys); err != nil {
		s.rollbackRelocations(ctx, relocations)
		if errors.Is(err, database.ErrWorkspaceFileMoveCycle) {
			return database.WorkspaceFile{}, ErrMoveCycle
		}
		if errors.Is(err, database.ErrInvalidWorkspaceFileParent) {
			return database.WorkspaceFile{}, ErrInvalidParent
		}
		if errors.Is(err, sql.ErrNoRows) {
			return database.WorkspaceFile{}, ErrNotFound
		}
		return database.WorkspaceFile{}, err
	}
	s.finishRelocations(ctx, planner, scope, file, relocations)
	if err := s.enrichFileWithCurrentContent(ctx, &updated); err != nil {
		return database.WorkspaceFile{}, err
	}
	return updated, nil
}

// Delete tombstones an authorized file/folder subtree and removes all tracked
// stored content for that subtree.
func (s *Service) Delete(ctx context.Context, scope Scope, fileID int64) error {
	lock := s.workspaceLock(scope.Workspace.ID)
	lock.Lock()
	defer lock.Unlock()

	file, err := s.authorizedFile(ctx, scope, access.PermWsFileDelete, fileID)
	if err != nil {
		return err
	}
	contents, err := s.db.ListWorkspaceFileSubtreeContents(ctx, file.ID)
	if err != nil {
		return err
	}
	contentStores := make(map[int64]filestore.Store, len(contents))
	for _, content := range contents {
		store, err := s.storeForContent(ctx, content)
		if err != nil {
			return err
		}
		contentStores[content.ID] = store
	}
	if err := s.db.DeleteWorkspaceFileTree(ctx, file.ID, scope.AccountID); err != nil {
		return err
	}
	for _, content := range contents {
		if err := contentStores[content.ID].Delete(ctx, content.StorageKey); err != nil {
			return err
		}
	}
	s.planner().Prune(ctx, s, scope, file)
	return nil
}

// ReadContent opens the current stored bytes for an authorized file.
func (s *Service) ReadContent(ctx context.Context, scope Scope, fileID int64) (ReadContentResult, error) {
	file, err := s.authorizedFile(ctx, scope, access.PermWsFileRead, fileID)
	if err != nil {
		return ReadContentResult{}, err
	}
	if file.ObjectType != database.FileObjectTypeFile {
		return ReadContentResult{}, ErrFolderContent
	}
	content, found, err := s.db.CurrentWorkspaceFileContent(ctx, file)
	if err != nil {
		return ReadContentResult{}, err
	}
	if !found {
		return ReadContentResult{}, ErrNotFound
	}
	store, err := s.storeForContent(ctx, content)
	if err != nil {
		return ReadContentResult{}, err
	}
	reader, object, err := store.Get(ctx, content.StorageKey)
	if err != nil {
		return ReadContentResult{}, err
	}
	return ReadContentResult{File: file, Object: object, Reader: reader}, nil
}

// WriteContent stores new bytes for an authorized file, enforcing If-Match for
// existing content and applying the configured revision strategy.
func (s *Service) WriteContent(ctx context.Context, scope Scope, fileID int64, expectedHash string, content io.Reader) (database.WorkspaceFileContent, error) {
	lock := s.workspaceLock(scope.Workspace.ID)
	lock.Lock()
	defer lock.Unlock()

	file, err := s.authorizedFile(ctx, scope, access.PermWsFileWrite, fileID)
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	if file.ObjectType != database.FileObjectTypeFile {
		return database.WorkspaceFileContent{}, ErrFolderContent
	}
	current, found, err := s.db.CurrentWorkspaceFileContent(ctx, file)
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	if found {
		store, err := s.storeForContent(ctx, current)
		if err != nil {
			return database.WorkspaceFileContent{}, err
		}
		reader, object, err := store.Get(ctx, current.StorageKey)
		if err != nil {
			return database.WorkspaceFileContent{}, err
		}
		reader.Close()
		expectedHash = strings.Trim(strings.TrimSpace(expectedHash), "\"")
		if expectedHash == "" {
			return database.WorkspaceFileContent{}, ErrPreconditionRequired
		}
		if expectedHash != object.ContentHash {
			return database.WorkspaceFileContent{}, ErrStaleContent
		}
	}

	planner := s.planner()
	storageKey, err := planner.ContentKey(ctx, s, scope, file, current, found)
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	activeBackendID := s.activeBackendID()
	store, err := s.storeForBackend(ctx, activeBackendID)
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	object, err := store.Put(ctx, storageKey, content)
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	externalModifiedAt := object.ModifiedTime
	saved, err := s.db.SaveWorkspaceFileContent(ctx, file.ID, scope.AccountID, database.WorkspaceFileContent{
		StorageBackendID:   activeBackendID,
		StorageKey:         object.Key,
		ContentHash:        object.ContentHash,
		SizeBytes:          object.SizeBytes,
		ExternalModifiedAt: &externalModifiedAt,
	}, planner.UsesRevisions(file))
	if err != nil {
		return database.WorkspaceFileContent{}, err
	}
	return saved, nil
}

// ValidName reports whether a file/folder name is safe as one path segment.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

// authorizedFile loads a file and enforces that it belongs to the requested
// workspace, tree, and actor.
func (s *Service) authorizedFile(ctx context.Context, scope Scope, permission string, fileID int64) (database.WorkspaceFile, error) {
	file, found, err := s.db.GetWorkspaceFile(ctx, fileID)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if !found || file.WorkspaceID != scope.Workspace.ID || file.Visibility != scope.Visibility {
		return database.WorkspaceFile{}, ErrNotFound
	}
	if file.Visibility == database.FileVisibilityPrivate && scope.Workspace.OwnerType == "org" {
		member, err := s.db.IsEffectiveWorkspaceMember(ctx, scope.OrgID, scope.Workspace.ID, scope.AccountID)
		if err != nil {
			return database.WorkspaceFile{}, err
		}
		if !member || file.OwnerAccountID == nil || *file.OwnerAccountID != scope.AccountID {
			return database.WorkspaceFile{}, ErrNotFound
		}
		return file, nil
	}
	ownerID, err := s.authorizeTree(ctx, scope, permission)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if ownerID != nil && (file.OwnerAccountID == nil || *ownerID != *file.OwnerAccountID) {
		return database.WorkspaceFile{}, ErrNotFound
	}
	return file, nil
}

// authorizeTree returns the owner account ID for private trees and nil for
// shared trees after enforcing the relevant access model.
func (s *Service) authorizeTree(ctx context.Context, scope Scope, permission string) (*int64, error) {
	if scope.Workspace.OwnerType == "space" {
		if scope.Visibility != database.FileVisibilityPrivate {
			return nil, ErrNotFound
		}
		ownerID := scope.AccountID
		return &ownerID, nil
	}
	switch scope.Visibility {
	case database.FileVisibilityPrivate:
		member, err := s.db.IsEffectiveWorkspaceMember(ctx, scope.OrgID, scope.Workspace.ID, scope.AccountID)
		if err != nil {
			return nil, err
		}
		if !member {
			return nil, ErrForbidden
		}
		ownerID := scope.AccountID
		return &ownerID, nil
	case database.FileVisibilityShared:
		if s.enforcer == nil || !s.enforcer.Can(ctx, scope.AccountID, scope.OrgID, scope.Workspace.OwnerType, "workspace", scope.Workspace.ID, permission) {
			return nil, ErrForbidden
		}
		return nil, nil
	default:
		return nil, ErrNotFound
	}
}

// validateParent checks that a parent folder belongs to the same workspace,
// visibility tree, and private owner as the file being created or moved.
func (s *Service) validateParent(ctx context.Context, scope Scope, parentID int64, ownerID *int64) error {
	parent, found, err := s.db.GetWorkspaceFile(ctx, parentID)
	if err != nil {
		return err
	}
	if !found || parent.WorkspaceID != scope.Workspace.ID || parent.ObjectType != database.FileObjectTypeFolder ||
		parent.Visibility != scope.Visibility || !sameNullableID(parent.OwnerAccountID, ownerID) {
		return ErrNotFound
	}
	return nil
}

// pathSegments converts the verified database parent chain into JSON-safe
// breadcrumb entries for browser clients.
func (s *Service) pathSegments(ctx context.Context, file database.WorkspaceFile) ([]PathSegment, error) {
	ancestors, err := s.db.WorkspaceFileAncestors(ctx, file)
	if err != nil {
		return nil, err
	}
	path := make([]PathSegment, 0, len(ancestors))
	for _, ancestor := range ancestors {
		path = append(path, PathSegment{ID: ancestor.ID, Name: ancestor.Name, ObjectType: ancestor.ObjectType})
	}
	return path, nil
}

// enrichFilesWithCurrentContent adds current content metadata to file entries
// without exposing storage keys or backend details.
func (s *Service) enrichFilesWithCurrentContent(ctx context.Context, files []database.WorkspaceFile) error {
	for i := range files {
		if err := s.enrichFileWithCurrentContent(ctx, &files[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enrichFileWithCurrentContent(ctx context.Context, file *database.WorkspaceFile) error {
	if file.ObjectType != database.FileObjectTypeFile {
		return nil
	}
	content, found, err := s.db.CurrentWorkspaceFileContent(ctx, *file)
	if err != nil {
		return err
	}
	if found {
		file.ContentHash = content.ContentHash
		file.ContentVersion = content.Version
		file.SizeBytes = content.SizeBytes
	}
	return nil
}

// workspaceLock serializes writes and structural mutations per workspace inside
// this process so file-mode paths do not race with content writes.
func (s *Service) workspaceLock(workspaceID int64) *sync.Mutex {
	lock, _ := s.locks.LoadOrStore("workspace-files:"+strconv.FormatInt(workspaceID, 10), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *Service) activeBackendID() string {
	if s.stores != nil {
		if backendID := s.stores.ActiveBackendID(); backendID != "" {
			return backendID
		}
	}
	if s.config.ActiveStorageBackendID != "" {
		return s.config.ActiveStorageBackendID
	}
	return database.DefaultFileStorageBackendID
}

func (s *Service) storeForContent(ctx context.Context, content database.WorkspaceFileContent) (filestore.Store, error) {
	backendID := content.StorageBackendID
	if backendID == "" {
		backendID = database.DefaultFileStorageBackendID
	}
	return s.storeForBackend(ctx, backendID)
}

func (s *Service) storeForBackend(ctx context.Context, backendID string) (filestore.Store, error) {
	if s.stores == nil {
		return nil, ErrStorageBackendUnavailable
	}
	store, err := s.stores.Store(ctx, backendID)
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, ErrStorageBackendUnavailable
	}
	return store, nil
}

type storagePlanner interface {
	ContentKey(context.Context, *Service, Scope, database.WorkspaceFile, database.WorkspaceFileContent, bool) (string, error)
	Relocations(context.Context, *Service, Scope, database.WorkspaceFile, database.WorkspaceFile) ([]relocation, error)
	Prune(context.Context, *Service, Scope, database.WorkspaceFile)
	UsesRevisions(database.WorkspaceFile) bool
}

func (s *Service) planner() storagePlanner {
	if s.config.StorageMode == StorageModeFile {
		return workspaceDirectoryPlanner{}
	}
	return objectStorePlanner{revisionPolicy: s.config.RevisionPolicy}
}

type objectStorePlanner struct {
	revisionPolicy string
}

// ContentKey returns an opaque object key independent of user-visible names.
func (p objectStorePlanner) ContentKey(_ context.Context, _ *Service, _ Scope, file database.WorkspaceFile, current database.WorkspaceFileContent, hasCurrent bool) (string, error) {
	suffix := "current"
	if p.UsesRevisions(file) {
		version := 1
		if hasCurrent {
			version = current.Version + 1
		}
		suffix = "versions/" + strconv.Itoa(version)
	}
	return path.Join("objects", strconv.FormatInt(file.WorkspaceID, 10), strconv.FormatInt(file.ID, 10), suffix), nil
}

// Relocations are unnecessary for opaque object keys because rename/move only
// changes metadata.
func (p objectStorePlanner) Relocations(context.Context, *Service, Scope, database.WorkspaceFile, database.WorkspaceFile) ([]relocation, error) {
	return nil, nil
}

// Prune is a no-op for opaque object storage.
func (p objectStorePlanner) Prune(context.Context, *Service, Scope, database.WorkspaceFile) {}

// UsesRevisions reports whether this file should write immutable versions.
func (p objectStorePlanner) UsesRevisions(file database.WorkspaceFile) bool {
	if p.revisionPolicy != RevisionPolicyVersioned {
		return false
	}
	if strings.HasPrefix(strings.ToLower(file.MediaType), "text/") {
		return true
	}
	switch strings.ToLower(file.FileKind) {
	case "query", "text", "text_document":
		return true
	}
	switch strings.ToLower(path.Ext(file.Name)) {
	case ".sql", ".txt", ".md", ".json", ".yaml", ".yml", ".toml":
		return true
	}
	return false
}

type workspaceDirectoryPlanner struct{}

// ContentKey mirrors the user-visible workspace file path on the byte store.
func (p workspaceDirectoryPlanner) ContentKey(ctx context.Context, svc *Service, scope Scope, file database.WorkspaceFile, _ database.WorkspaceFileContent, _ bool) (string, error) {
	names, err := svc.db.WorkspaceFilePath(ctx, file)
	if err != nil {
		return "", err
	}
	segments := []string{}
	if scope.Workspace.OwnerType == "space" {
		segments = []string{"personal", strconv.FormatInt(scope.Workspace.OwnerID, 10), "workspaces", workspaceStorageSegment(scope.Workspace)}
	} else {
		segments = []string{"organizations", scope.OrgSlug, "workspaces", workspaceStorageSegment(scope.Workspace)}
	}
	if file.Visibility == database.FileVisibilityPrivate {
		segments = append(segments, "my-files", strconv.FormatInt(*file.OwnerAccountID, 10))
	} else {
		segments = append(segments, "shared")
	}
	segments = append(segments, names...)
	return path.Join(segments...), nil
}

// Relocations plans content-key changes for all tracked descendants when the
// visible path of a file or folder changes.
func (p workspaceDirectoryPlanner) Relocations(ctx context.Context, svc *Service, scope Scope, current, updated database.WorkspaceFile) ([]relocation, error) {
	if current.Name == updated.Name && sameNullableID(current.ParentID, updated.ParentID) {
		return nil, nil
	}
	oldRoot, err := p.ContentKey(ctx, svc, scope, current, database.WorkspaceFileContent{}, false)
	if err != nil {
		return nil, err
	}
	newRoot, err := p.ContentKey(ctx, svc, scope, updated, database.WorkspaceFileContent{}, false)
	if err != nil {
		return nil, err
	}
	contents, err := svc.db.ListWorkspaceFileSubtreeContents(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	relocations := make([]relocation, 0, len(contents))
	for _, content := range contents {
		if content.StorageKey != oldRoot && !strings.HasPrefix(content.StorageKey, oldRoot+"/") {
			continue
		}
		backendID := content.StorageBackendID
		if backendID == "" {
			backendID = database.DefaultFileStorageBackendID
		}
		relocations = append(relocations, relocation{
			contentID: content.ID,
			backendID: backendID,
			oldKey:    content.StorageKey,
			newKey:    newRoot + strings.TrimPrefix(content.StorageKey, oldRoot),
		})
	}
	return relocations, nil
}

// Prune removes empty visible directories left behind after moves/deletes.
func (p workspaceDirectoryPlanner) Prune(ctx context.Context, svc *Service, scope Scope, file database.WorkspaceFile) {
	store, err := svc.storeForBackend(ctx, svc.activeBackendID())
	if err != nil {
		return
	}
	pruner, ok := store.(filestore.EmptyDirectoryPruner)
	if !ok {
		return
	}
	key, err := p.ContentKey(ctx, svc, scope, file, database.WorkspaceFileContent{}, false)
	if err != nil {
		return
	}
	if file.ObjectType == database.FileObjectTypeFile {
		key = path.Dir(key)
	}
	_ = pruner.PruneEmptyDirectories(ctx, key)
}

// UsesRevisions is false for visible directories because the path is meant to be
// directly editable by users and hidden history is not implemented there.
func (p workspaceDirectoryPlanner) UsesRevisions(database.WorkspaceFile) bool {
	return false
}

type relocation struct {
	contentID int64
	backendID string
	oldKey    string
	newKey    string
	copied    bool
}

// stageRelocations copies tracked bytes to destination keys before metadata is
// updated, allowing rollback if the database update fails.
func (s *Service) stageRelocations(ctx context.Context, relocations []relocation) error {
	for i := range relocations {
		if relocations[i].oldKey == relocations[i].newKey {
			continue
		}
		store, err := s.storeForBackend(ctx, relocations[i].backendID)
		if err != nil {
			s.rollbackRelocations(ctx, relocations[:i])
			return err
		}
		existing, _, err := store.Get(ctx, relocations[i].newKey)
		if err == nil {
			existing.Close()
			s.rollbackRelocations(ctx, relocations[:i])
			return ErrStorageDestinationExists
		}
		if !errors.Is(err, os.ErrNotExist) {
			s.rollbackRelocations(ctx, relocations[:i])
			return err
		}
		reader, _, err := store.Get(ctx, relocations[i].oldKey)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			s.rollbackRelocations(ctx, relocations[:i])
			return err
		}
		_, err = store.Put(ctx, relocations[i].newKey, reader)
		reader.Close()
		if err != nil {
			s.rollbackRelocations(ctx, relocations[:i])
			return err
		}
		relocations[i].copied = true
	}
	return nil
}

// rollbackRelocations removes staged destination copies after a failed metadata
// update.
func (s *Service) rollbackRelocations(ctx context.Context, relocations []relocation) {
	for _, relocation := range relocations {
		if relocation.copied {
			if store, err := s.storeForBackend(ctx, relocation.backendID); err == nil {
				_ = store.Delete(ctx, relocation.newKey)
			}
		}
	}
}

// finishRelocations removes old tracked byte keys after metadata points at the
// new keys.
func (s *Service) finishRelocations(ctx context.Context, planner storagePlanner, scope Scope, original database.WorkspaceFile, relocations []relocation) {
	for _, relocation := range relocations {
		if relocation.copied {
			if store, err := s.storeForBackend(ctx, relocation.backendID); err == nil {
				_ = store.Delete(ctx, relocation.oldKey)
			}
		}
	}
	planner.Prune(ctx, s, scope, original)
}

func workspaceStorageSegment(ws database.Workspace) string {
	return strconv.FormatInt(ws.ID, 10) + "-" + slugify(ws.Name)
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func sameNullableID(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
