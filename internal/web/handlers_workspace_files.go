package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

type workspaceFilesResponse struct {
	Files []files.File `json:"files"`
}

func (app *application) listPrivateWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.listWorkspaceFiles(w, r, files.VisibilityPrivate)
}

func (app *application) listSharedWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.listWorkspaceFiles(w, r, files.VisibilityShared)
}

func (app *application) browsePrivateWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.browseWorkspaceFiles(w, r, files.VisibilityPrivate)
}

func (app *application) browseSharedWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.browseWorkspaceFiles(w, r, files.VisibilityShared)
}

// browseWorkspaceFiles returns the editor browser snapshot for root, folder, or file.
func (app *application) browseWorkspaceFiles(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.optionalPositiveID(w, r, "file_id")
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	result, err := app.workspaceFileService().Browser(r.Context(), principal, scope, fileID)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listRecentPrivateWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.listRecentWorkspaceFiles(w, r, files.VisibilityPrivate)
}

func (app *application) listRecentSharedWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.listRecentWorkspaceFiles(w, r, files.VisibilityShared)
}

// listRecentWorkspaceFiles returns recently changed openable files for the route-selected tree.
func (app *application) listRecentWorkspaceFiles(w http.ResponseWriter, r *http.Request, visibility string) {
	limit, ok := app.optionalPositiveLimit(w, r, "limit", 20, 100)
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	files, err := app.workspaceFileService().Recent(r.Context(), principal, scope, limit)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, workspaceFilesResponse{Files: files}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) searchPrivateWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.searchWorkspaceFiles(w, r, files.VisibilityPrivate)
}

func (app *application) searchSharedWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	app.searchWorkspaceFiles(w, r, files.VisibilityShared)
}

// searchWorkspaceFiles scans the route-selected file tree for a content match.
func (app *application) searchWorkspaceFiles(w http.ResponseWriter, r *http.Request, visibility string) {
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	query := r.URL.Query().Get("q")
	result, err := app.workspaceFileService().Search(r.Context(), principal, scope, files.SearchInput{Query: query})
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

// listWorkspaceFiles lists direct children for the route-selected file tree.
func (app *application) listWorkspaceFiles(w http.ResponseWriter, r *http.Request, visibility string) {
	parentID, ok := app.optionalPositiveID(w, r, "parent_id")
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	files, err := app.workspaceFileService().List(r.Context(), principal, scope, parentID)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, workspaceFilesResponse{Files: files}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createPrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.createWorkspaceFile(w, r, files.VisibilityPrivate)
}

func (app *application) createSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.createWorkspaceFile(w, r, files.VisibilityShared)
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
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	file, err := app.workspaceFileService().Create(r.Context(), principal, scope, files.CreateInput{
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
	app.logInfo(r, "workspace file created", slog.Int64("workspace_id", file.WorkspaceID), slog.Int64("file_id", file.ID), slog.String("visibility", file.Visibility), slog.String("object_type", file.ObjectType))
	if err := response.JSON(w, http.StatusCreated, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getPrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFile(w, r, files.VisibilityPrivate)
}

func (app *application) getSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFile(w, r, files.VisibilityShared)
}

// getWorkspaceFile returns one file/folder's metadata from the selected tree.
func (app *application) getWorkspaceFile(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	file, err := app.workspaceFileService().Get(r.Context(), principal, scope, fileID)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updatePrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFile(w, r, files.VisibilityPrivate)
}

func (app *application) updateSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFile(w, r, files.VisibilityShared)
}

func (app *application) duplicatePrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, files.VisibilityPrivate)
	if !ok {
		return
	}
	service, err := app.workspaceFileServiceForRequest(r, principal, scope)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	file, err := service.Duplicate(
		r.Context(),
		principal,
		scope,
		fileID,
		files.DuplicateInput{Name: input.Name},
	)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A file or folder with this name already exists here.")
			return
		}
		app.workspaceFileError(w, r, err)
		return
	}
	app.logInfo(r, "workspace file duplicated", slog.Int64("workspace_id", file.WorkspaceID), slog.Int64("file_id", file.ID), slog.Int64("source_file_id", fileID))
	if err := response.JSON(w, http.StatusCreated, file); err != nil {
		app.serverError(w, r, err)
	}
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
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	file, err := app.workspaceFileService().Update(r.Context(), principal, scope, fileID, parsed)
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "name", "A file or folder with this name already exists here.")
			return
		}
		app.workspaceFileError(w, r, err)
		return
	}
	app.logInfo(r, "workspace file updated", slog.Int64("workspace_id", file.WorkspaceID), slog.Int64("file_id", file.ID), slog.String("visibility", file.Visibility), slog.String("object_type", file.ObjectType))
	if err := response.JSON(w, http.StatusOK, file); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) deletePrivateWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.deleteWorkspaceFile(w, r, files.VisibilityPrivate)
}

func (app *application) deleteSharedWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	app.deleteWorkspaceFile(w, r, files.VisibilityShared)
}

// deleteWorkspaceFile recursively tombstones a file/folder subtree.
func (app *application) deleteWorkspaceFile(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	if err := app.workspaceFileService().Delete(r.Context(), principal, scope, fileID); err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	app.logInfo(r, "workspace file deleted", slog.Int64("file_id", fileID), slog.String("visibility", visibility))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getPrivateWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFileContent(w, r, files.VisibilityPrivate)
}

func (app *application) getSharedWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.getWorkspaceFileContent(w, r, files.VisibilityShared)
}

// getWorkspaceFileContent streams the current bytes for a file.
func (app *application) getWorkspaceFileContent(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	result, err := app.workspaceFileService().ReadContent(r.Context(), principal, scope, fileID)
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
	app.updateWorkspaceFileContent(w, r, files.VisibilityPrivate)
}

func (app *application) updateSharedWorkspaceFileContent(w http.ResponseWriter, r *http.Request) {
	app.updateWorkspaceFileContent(w, r, files.VisibilityShared)
}

// updateWorkspaceFileContent stores new bytes for a file.
func (app *application) updateWorkspaceFileContent(w http.ResponseWriter, r *http.Request, visibility string) {
	fileID, ok := app.workspaceFileID(w, r)
	if !ok {
		return
	}
	principal, scope, ok := app.workspaceFileRequest(w, r, visibility)
	if !ok {
		return
	}
	service, err := app.workspaceFileServiceForRequest(r, principal, scope)
	if err != nil {
		app.workspaceFileError(w, r, err)
		return
	}
	saved, err := service.WriteContent(
		r.Context(),
		principal,
		scope,
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
	app.logInfo(r, "workspace file content updated", slog.Int64("file_content_id", saved.ID), slog.Int64("file_id", saved.FileID), slog.Int64("size_bytes", saved.SizeBytes), slog.String("storage_backend_id", saved.StorageBackendID))
	if err := response.JSON(w, http.StatusOK, saved); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) workspaceFileServiceForRequest(r *http.Request, p access.Principal, scope files.Scope) (*files.Service, error) {
	return app.services.FilesForPrincipal(r.Context(), p, scope.OrgID, scope.WorkspaceID)
}

// workspaceFileRequest returns the request principal and the routed file scope.
func (app *application) workspaceFileRequest(w http.ResponseWriter, r *http.Request, visibility string) (access.Principal, files.Scope, bool) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return access.Principal{}, files.Scope{}, false
	}
	return principal, files.Scope{
		OrgID:       contextGetPathIDs(r).OrgID,
		WorkspaceID: contextGetPathIDs(r).WorkspaceID,
		Visibility:  visibility,
	}, true
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
	case errors.Is(err, files.ErrStorageBackendUnavailable):
		app.logWarn(r, "workspace file storage unavailable")
		app.errorMessage(w, r, http.StatusServiceUnavailable, "File storage backend is unavailable.", nil)
	case errors.Is(err, files.ErrInvalidName):
		app.failedValidation(w, r, fieldErrors(map[string]string{"name": "Name must be a valid file or folder name."}))
	case errors.Is(err, files.ErrInvalidObjectType):
		app.failedValidation(w, r, fieldErrors(map[string]string{"object_type": "Object type must be file or folder."}))
	case errors.Is(err, files.ErrInvalidSearchQuery):
		app.failedValidation(w, r, fieldErrors(map[string]string{"q": "Search query must be at least 2 characters."}))
	case files.IsInvalidParent(err):
		app.failedValidation(w, r, fieldErrors(map[string]string{"parent_id": "Parent folder is invalid."}))
	case files.IsMoveCycle(err):
		app.failedValidation(w, r, fieldErrors(map[string]string{"parent_id": "A folder cannot be moved inside its descendant."}))
	case errors.Is(err, files.ErrMissingUpdate):
		app.failedValidation(w, r, fieldErrors(map[string]string{"file": "Name or parent ID must be provided."}))
	case errors.Is(err, files.ErrFolderContent):
		app.failedValidation(w, r, fieldErrors(map[string]string{"file": "Folder content cannot be updated."}))
	case errors.Is(err, files.ErrFolderDuplicate):
		app.failedValidation(w, r, fieldErrors(map[string]string{"file": "Folders cannot be duplicated."}))
	case errors.Is(err, files.ErrPreconditionRequired):
		app.errorMessage(w, r, http.StatusPreconditionRequired, "If-Match is required when updating existing file content.", nil)
	case errors.Is(err, files.ErrStaleContent):
		app.logWarn(r, "workspace file stale content update blocked")
		app.errorMessage(w, r, http.StatusConflict, "The file has changed since it was opened. Reload it before saving.", nil)
	case errors.Is(err, files.ErrStorageDestinationExists):
		app.logWarn(r, "workspace file storage destination conflict")
		app.errorMessage(w, r, http.StatusConflict, "A file already exists at the destination path on storage.", nil)
	default:
		app.serverError(w, r, err)
	}
}

func (app *application) optionalPositiveID(w http.ResponseWriter, r *http.Request, name string) (*int64, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		app.failedValidation(w, r, fieldErrors(map[string]string{name: "Value must be a positive integer."}))
		return nil, false
	}
	return &id, true
}

func (app *application) optionalPositiveLimit(w http.ResponseWriter, r *http.Request, name string, defaultValue, maxValue int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return defaultValue, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		app.failedValidation(w, r, fieldErrors(map[string]string{name: "Value must be a positive integer."}))
		return 0, false
	}
	if limit > maxValue {
		limit = maxValue
	}
	return limit, true
}

func quoteETag(hash string) string {
	return strconv.Quote(hash)
}
