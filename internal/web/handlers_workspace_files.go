package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) listPrivateWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.listWorkspaceFiles(w, r, database.FileVisibilityPrivate)
}

func (app *application) listSharedWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.listWorkspaceFiles(w, r, database.FileVisibilityShared)
}

// listWorkspaceFiles lists direct children for the route-selected file tree.
func (app *application) listWorkspaceFiles(w http.ResponseWriter, r *http.Request, visibility string) {
	parentID, ok := optionalPositiveID(w, r, "parent_id")
	if !ok {
		return
	}
	files, err := app.workspaceFileService().List(r.Context(), app.workspaceFileScope(r, visibility), parentID)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, files); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createPrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.createWorkspaceFile(w, r, database.FileVisibilityPrivate)
}

func (app *application) createSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.createWorkspaceFile(w, r, database.FileVisibilityShared)
}

// createWorkspaceFile creates a file/folder in the route-selected file tree.
func (app *application) createWorkspaceFile(w http.ResponseWriter, r *http.Request, visibility string) {
	var input struct {
		Name       string              `json:"name"`
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
	file, err := app.workspaceFileService().Create(r.Context(), app.workspaceFileScope(r, visibility), files.CreateInput{
		Name:       input.Name,
		ObjectType: input.ObjectType,
		ParentID:   input.ParentID,
		MediaType:  input.MediaType,
		FileKind:   input.FileKind,
	})
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A file or folder with this name already exists here.")
			return
		}
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusCreated, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getPrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFile(w, r, database.FileVisibilityPrivate)
}

func (app *application) getSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFile(w, r, database.FileVisibilityShared)
}

// getWorkspaceFile returns one file/folder's metadata from the selected tree.
func (app *application) getWorkspaceFile(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	file, err := app.workspaceFileService().Get(r.Context(), app.workspaceFileScope(r, visibility), fileID)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updatePrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFile(w, r, database.FileVisibilityPrivate)
}

func (app *application) updateSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFile(w, r, database.FileVisibilityShared)
}

// updateWorkspaceFile renames and/or moves a file/folder in the selected tree.
func (app *application) updateWorkspaceFile(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	var input struct {
		Name     *string         `json:"name"`
		ParentID json.RawMessage `json:"parent_id"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	parsed := files.UpdateInput{Name: input.Name, ParentIDSet: input.ParentID != nil}
	if input.ParentID != nil && string(input.ParentID) != "null" {
		var parentID int64
		if err := json.Unmarshal(input.ParentID, &parentID); err != nil || parentID < 1 {
			app.failedValidation(w, r, fieldErrors(map[string]string{"parent_id": "Parent ID must be a positive integer or null."}))
			return
		}
		parsed.ParentID = &parentID
	}
	file, err := app.workspaceFileService().Update(r.Context(), app.workspaceFileScope(r, visibility), fileID, parsed)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A file or folder with this name already exists here.")
			return
		}
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deletePrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.deleteWorkspaceFile(w, r, database.FileVisibilityPrivate)
}

func (app *application) deleteSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.deleteWorkspaceFile(w, r, database.FileVisibilityShared)
}

// deleteWorkspaceFile recursively tombstones a file/folder subtree.
func (app *application) deleteWorkspaceFile(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	if err := app.workspaceFileService().Delete(r.Context(), app.workspaceFileScope(r, visibility), fileID); err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getPrivateWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFileContent(w, r, database.FileVisibilityPrivate)
}

func (app *application) getSharedWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFileContent(w, r, database.FileVisibilityShared)
}

// getWorkspaceFileContent streams the current bytes for a file.
func (app *application) getWorkspaceFileContent(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	result, err := app.workspaceFileService().ReadContent(r.Context(), app.workspaceFileScope(r, visibility), fileID)
	if err != nil {
		if errors.Is(err, files.ErrFolderContent) {
			app.notFound(w, r)
			return
		}
		app.workspaceFileError(w, r, err)
		return
	}
	defer result.Reader.Close()
	w.Header().Set("ETag", quoteETag(result.Object.ContentHash))
	w.Header().Set("X-Content-Hash", result.Object.ContentHash)
	if result.File.MediaType != "" {
		w.Header().Set("Content-Type", result.File.MediaType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, result.Reader); err != nil {
		app.reportServerError(r, err)
	}
}

func (app *application) updatePrivateWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFileContent(w, r, database.FileVisibilityPrivate)
}

func (app *application) updateSharedWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFileContent(w, r, database.FileVisibilityShared)
}

// updateWorkspaceFileContent stores new bytes for a file.
func (app *application) updateWorkspaceFileContent(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	saved, err := app.workspaceFileService().WriteContent(
		r.Context(),
		app.workspaceFileScope(r, visibility),
		fileID,
		r.Header.Get("If-Match"),
		http.MaxBytesReader(w, r.Body, 100<<20),
	)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			app.errorMessage(w, r, http.StatusRequestEntityTooLarge, "File content exceeds the maximum upload size.", nil)
			return
		}
		app.workspaceFileError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteETag(saved.ContentHash))
	if err := response.JSON(w, http.StatusOK, saved); err != nil {
		app.serverError(w, r, err)
	}
}

// workspaceFileService builds the file domain service from current app state.
func (app *application) workspaceFileService() *files.Service {
	return files.New(app.db, app.fileStore, app.enforcer, files.Config{
		StorageMode:    app.config.Files.StorageMode,
		RevisionPolicy: app.config.Files.Revisions.DefaultPolicy,
	}, &app.fileLocks)
}

// workspaceFileScope converts request context into domain scope.
func (app *application) workspaceFileScope(r *http.Request, visibility string) files.Scope {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	return files.Scope{
		AccountID:  account.ID,
		OrgID:      org.ID,
		OrgSlug:    org.Slug,
		Workspace:  contextGetWorkspace(r),
		Visibility: visibility,
	}
}

// workspaceFileID parses the path file ID.
func (app *application) workspaceFileID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "file_id"), 10, 64)
	if err != nil || id < 1 {
		app.notFound(w, r)
		return 0, false
	}
	return id, true
}

// workspaceFileError maps domain errors to stable API responses.
func (app *application) workspaceFileError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, files.ErrForbidden):
		app.notPermitted(w, r)
	case errors.Is(err, files.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, files.ErrInvalidName):
		app.failedValidation(w, r, fieldErrors(map[string]string{"name": "Name must be a valid file or folder name."}))
	case errors.Is(err, files.ErrInvalidObjectType):
		app.failedValidation(w, r, fieldErrors(map[string]string{"object_type": "Object type must be file or folder."}))
	case errors.Is(err, files.ErrInvalidParent), errors.Is(err, database.ErrInvalidWorkspaceFileParent):
		app.failedValidation(w, r, fieldErrors(map[string]string{"parent_id": "Parent folder is invalid."}))
	case errors.Is(err, files.ErrMoveCycle), errors.Is(err, database.ErrWorkspaceFileMoveCycle):
		app.failedValidation(w, r, fieldErrors(map[string]string{"parent_id": "A folder cannot be moved inside its descendant."}))
	case errors.Is(err, files.ErrMissingUpdate):
		app.failedValidation(w, r, fieldErrors(map[string]string{"file": "Name or parent ID must be provided."}))
	case errors.Is(err, files.ErrFolderContent):
		app.failedValidation(w, r, fieldErrors(map[string]string{"file": "Folder content cannot be updated."}))
	case errors.Is(err, files.ErrPreconditionRequired):
		app.errorMessage(w, r, http.StatusPreconditionRequired, "If-Match is required when updating existing file content.", nil)
	case errors.Is(err, files.ErrStaleContent):
		app.errorMessage(w, r, http.StatusConflict, "The file has changed since it was opened. Reload it before saving.", nil)
	case errors.Is(err, files.ErrStorageDestinationExists):
		app.errorMessage(w, r, http.StatusConflict, "A file already exists at the destination path on storage.", nil)
	default:
		app.serverError(w, r, err)
	}
}

func optionalPositiveID(w http.ResponseWriter, r *http.Request, name string) (*int64, bool) {
	raw := r.URL.Query().Get(name)
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

func quoteETag(hash string) string {
	return strconv.Quote(hash)
}
