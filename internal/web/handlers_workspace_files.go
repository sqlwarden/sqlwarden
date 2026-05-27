package web

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	if !app.filesAvailable(w, r) {
		return
	}
	visibility, ownerID, ok := app.requestedFileTree(w, r, access.PermWsFileRead)
	if !ok {
		return
	}
	parentID, ok := optionalPositiveID(w, r, "parent_id")
	if !ok {
		return
	}
	if parentID != nil && !app.validateFileParent(w, r, *parentID, visibility, ownerID) {
		return
	}
	files, err := app.db.ListWorkspaceFiles(r.Context(), contextGetWorkspace(r).ID, visibility, ownerID, parentID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, files); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if !app.filesAvailable(w, r) {
		return
	}
	var input struct {
		Name       string              `json:"name"`
		Visibility string              `json:"visibility"`
		ObjectType string              `json:"object_type"`
		ParentID   *int64              `json:"parent_id"`
		MediaType  string              `json:"media_type"`
		FileKind   string              `json:"file_kind"`
		V          validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	if input.ObjectType == "" {
		input.ObjectType = database.FileObjectTypeFile
	}
	input.V.CheckField(validFileName(input.Name), "name", "Name must be a valid file or folder name.")
	input.V.CheckField(input.ObjectType == database.FileObjectTypeFile || input.ObjectType == database.FileObjectTypeFolder, "object_type", "Object type must be file or folder.")
	if input.ObjectType == database.FileObjectTypeFolder {
		input.V.CheckField(input.MediaType == "", "media_type", "Folders cannot specify a media type.")
		input.V.CheckField(input.FileKind == "", "file_kind", "Folders cannot specify a file kind.")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	visibility, ownerID, ok := app.authorizeFileTree(w, r, input.Visibility, access.PermWsFileCreate)
	if !ok {
		return
	}
	if input.ParentID != nil && !app.validateFileParent(w, r, *input.ParentID, visibility, ownerID) {
		return
	}
	account := contextGetAccount(r)
	file := database.WorkspaceFile{
		WorkspaceID:    contextGetWorkspace(r).ID,
		ParentID:       input.ParentID,
		Visibility:     visibility,
		OwnerAccountID: ownerID,
		ObjectType:     input.ObjectType,
		Name:           strings.TrimSpace(input.Name),
		MediaType:      strings.TrimSpace(input.MediaType),
		FileKind:       strings.TrimSpace(input.FileKind),
		CreatedBy:      account.ID,
		UpdatedBy:      account.ID,
	}
	if err := app.db.InsertWorkspaceFile(r.Context(), &file); err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A file or folder with this name already exists here.")
			return
		}
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusCreated, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	file, ok := app.authorizedWorkspaceFile(w, r, access.PermWsFileRead)
	if !ok {
		return
	}
	content, found, err := app.db.CurrentWorkspaceFileContent(r.Context(), file)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if found {
		file.ContentHash = content.ContentHash
		file.ContentVersion = content.Version
		file.SizeBytes = content.SizeBytes
	}
	if err := response.JSON(w, http.StatusOK, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	file, ok := app.authorizedWorkspaceFile(w, r, access.PermWsFileRead)
	if !ok {
		return
	}
	if file.ObjectType != database.FileObjectTypeFile {
		app.notFound(w, r)
		return
	}
	content, found, err := app.db.CurrentWorkspaceFileContent(r.Context(), file)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	reader, object, err := app.fileStore.Get(r.Context(), content.StorageKey)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	defer reader.Close()
	w.Header().Set("ETag", quoteETag(object.ContentHash))
	w.Header().Set("X-Content-Hash", object.ContentHash)
	if file.MediaType != "" {
		w.Header().Set("Content-Type", file.MediaType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, reader); err != nil {
		app.reportServerError(r, err)
	}
}

func (app *application) updateWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	file, ok := app.authorizedWorkspaceFile(w, r, access.PermWsFileWrite)
	if !ok {
		return
	}
	if file.ObjectType != database.FileObjectTypeFile {
		app.failedValidation(w, r, fieldErrors(map[string]string{"file": "Folder content cannot be updated."}))
		return
	}
	lock := app.workspaceFileLock(file.ID)
	lock.Lock()
	defer lock.Unlock()

	current, found, err := app.db.CurrentWorkspaceFileContent(r.Context(), file)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if found {
		reader, object, err := app.fileStore.Get(r.Context(), current.StorageKey)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		reader.Close()
		expected := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), "\"")
		if expected == "" {
			app.errorMessage(w, r, http.StatusPreconditionRequired, "If-Match is required when updating existing file content.", nil)
			return
		}
		if expected != object.ContentHash {
			app.errorMessage(w, r, http.StatusConflict, "The file has changed since it was opened. Reload it before saving.", nil)
			return
		}
	}

	storageKey, err := app.workspaceFileStorageKey(r, file, current, found)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	object, err := app.fileStore.Put(r.Context(), storageKey, http.MaxBytesReader(w, r.Body, 100<<20))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			app.errorMessage(w, r, http.StatusRequestEntityTooLarge, "File content exceeds the maximum upload size.", nil)
			return
		}
		app.serverError(w, r, err)
		return
	}
	account := contextGetAccount(r)
	externalModifiedAt := object.ModifiedTime
	content := database.WorkspaceFileContent{
		StorageKey:         object.Key,
		ContentHash:        object.ContentHash,
		SizeBytes:          object.SizeBytes,
		ExternalModifiedAt: &externalModifiedAt,
	}
	versioned := app.workspaceFileUsesRevisions(file)
	saved, err := app.db.SaveWorkspaceFileContent(r.Context(), file.ID, account.ID, content, versioned)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteETag(saved.ContentHash))
	if err := response.JSON(w, http.StatusOK, saved); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) requestedFileTree(w http.ResponseWriter, r *http.Request, sharedPermission string) (string, *int64, bool) {
	return app.authorizeFileTree(w, r, strings.TrimSpace(r.URL.Query().Get("visibility")), sharedPermission)
}

func (app *application) authorizeFileTree(w http.ResponseWriter, r *http.Request, visibility, sharedPermission string) (string, *int64, bool) {
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)
	if ws.OwnerType == "space" {
		if visibility != "" && visibility != database.FileVisibilityPrivate {
			app.failedValidation(w, r, fieldErrors(map[string]string{"visibility": "Personal workspace files must be private."}))
			return "", nil, false
		}
		ownerID := account.ID
		return database.FileVisibilityPrivate, &ownerID, true
	}
	if visibility == "" {
		visibility = database.FileVisibilityPrivate
	}
	switch visibility {
	case database.FileVisibilityPrivate:
		member, err := app.db.IsEffectiveWorkspaceMember(r.Context(), contextGetOrg(r).ID, ws.ID, account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return "", nil, false
		}
		if !member {
			app.notPermitted(w, r)
			return "", nil, false
		}
		ownerID := account.ID
		return visibility, &ownerID, true
	case database.FileVisibilityShared:
		if !app.enforcer.Can(r.Context(), account.ID, contextGetOrg(r).ID, ws.OwnerType, "workspace", ws.ID, sharedPermission) {
			app.notPermitted(w, r)
			return "", nil, false
		}
		return visibility, nil, true
	default:
		app.failedValidation(w, r, fieldErrors(map[string]string{"visibility": "Visibility must be private or shared."}))
		return "", nil, false
	}
}

func (app *application) authorizedWorkspaceFile(w http.ResponseWriter, r *http.Request, sharedPermission string) (database.WorkspaceFile, bool) {
	if !app.filesAvailable(w, r) {
		return database.WorkspaceFile{}, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "file_id"), 10, 64)
	if err != nil || id < 1 {
		app.notFound(w, r)
		return database.WorkspaceFile{}, false
	}
	file, found, err := app.db.GetWorkspaceFile(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return database.WorkspaceFile{}, false
	}
	if !found || file.WorkspaceID != contextGetWorkspace(r).ID {
		app.notFound(w, r)
		return database.WorkspaceFile{}, false
	}
	ws := contextGetWorkspace(r)
	account := contextGetAccount(r)
	if file.Visibility == database.FileVisibilityPrivate && ws.OwnerType == "org" {
		member, err := app.db.IsEffectiveWorkspaceMember(r.Context(), contextGetOrg(r).ID, ws.ID, account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return database.WorkspaceFile{}, false
		}
		if !member || file.OwnerAccountID == nil || *file.OwnerAccountID != account.ID {
			app.notFound(w, r)
			return database.WorkspaceFile{}, false
		}
		return file, true
	}
	visibility, ownerID, ok := app.authorizeFileTree(w, r, file.Visibility, sharedPermission)
	if !ok || visibility != file.Visibility || (ownerID != nil && (file.OwnerAccountID == nil || *ownerID != *file.OwnerAccountID)) {
		if ok {
			app.notFound(w, r)
		}
		return database.WorkspaceFile{}, false
	}
	return file, true
}

func (app *application) validateFileParent(w http.ResponseWriter, r *http.Request, id int64, visibility string, ownerID *int64) bool {
	parent, found, err := app.db.GetWorkspaceFile(r.Context(), id)
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	if !found || parent.WorkspaceID != contextGetWorkspace(r).ID || parent.ObjectType != database.FileObjectTypeFolder ||
		parent.Visibility != visibility || !matchingOwner(parent.OwnerAccountID, ownerID) {
		app.notFound(w, r)
		return false
	}
	return true
}

func (app *application) filesAvailable(w http.ResponseWriter, r *http.Request) bool {
	if !app.config.Files.Enabled || app.fileStore == nil {
		app.notFound(w, r)
		return false
	}
	return true
}

func (app *application) workspaceFileStorageKey(r *http.Request, file database.WorkspaceFile, current database.WorkspaceFileContent, hasCurrent bool) (string, error) {
	if app.config.Files.StorageModel == FilesStorageModelObjectStore {
		suffix := "current"
		if app.workspaceFileUsesRevisions(file) {
			version := 1
			if hasCurrent {
				version = current.Version + 1
			}
			suffix = "versions/" + strconv.Itoa(version)
		}
		return path.Join("objects", strconv.FormatInt(file.WorkspaceID, 10), strconv.FormatInt(file.ID, 10), suffix), nil
	}
	names, err := app.db.WorkspaceFilePath(r.Context(), file)
	if err != nil {
		return "", err
	}
	ws := contextGetWorkspace(r)
	var segments []string
	if ws.OwnerType == "space" {
		segments = []string{"personal", strconv.FormatInt(ws.OwnerID, 10), "workspaces", workspaceStorageSegment(ws)}
	} else {
		segments = []string{"organizations", contextGetOrg(r).Slug, "workspaces", workspaceStorageSegment(ws)}
	}
	if file.Visibility == database.FileVisibilityPrivate {
		segments = append(segments, "my-files", strconv.FormatInt(*file.OwnerAccountID, 10))
	} else {
		segments = append(segments, "shared")
	}
	segments = append(segments, names...)
	return path.Join(segments...), nil
}

func workspaceStorageSegment(ws database.Workspace) string {
	return strconv.FormatInt(ws.ID, 10) + "-" + slugify(ws.Name)
}

func optionalPositiveID(w http.ResponseWriter, r *http.Request, name string) (*int64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		response.JSON(w, http.StatusUnprocessableEntity, fieldErrors(map[string]string{name: "Value must be a positive integer."}))
		return nil, false
	}
	return &id, true
}

func validFileName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

func matchingOwner(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func quoteETag(hash string) string {
	return fmt.Sprintf("%q", hash)
}

func (app *application) workspaceFileUsesRevisions(file database.WorkspaceFile) bool {
	if app.config.Files.StorageModel != FilesStorageModelObjectStore || app.config.Files.Revisions.DefaultPolicy != FilesRevisionPolicyVersioned {
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

func (app *application) workspaceFileLock(fileID int64) *sync.Mutex {
	lock, _ := app.fileLocks.LoadOrStore(fileID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}
