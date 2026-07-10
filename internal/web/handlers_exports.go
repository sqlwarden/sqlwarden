package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

const exportProgressEveryRows int64 = 10000

type exportRequest struct {
	SQL      string              `json:"sql"`
	Format   string              `json:"format"`
	Filename string              `json:"filename"`
	V        validator.Validator `json:"-"`
}

type exportJobInput struct {
	AccountID    int64  `json:"account_id"`
	OrgID        int64  `json:"org_id"`
	WorkspaceID  int64  `json:"workspace_id"`
	ConnectionID int64  `json:"connection_id"`
	SQL          string `json:"sql"`
	Format       string `json:"format"`
	Filename     string `json:"filename,omitempty"`
}

type exportJobOutput struct {
	FileID    int64  `json:"file_id"`
	Filename  string `json:"filename"`
	Format    string `json:"format"`
	RowCount  int64  `json:"row_count"`
	ByteCount int64  `json:"byte_count"`
}

func (app *application) createConnectionExport(w http.ResponseWriter, r *http.Request) {
	input, ok := app.decodeExportRequest(w, r)
	if !ok {
		return
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	conn := contextGetConnection(r)
	if !app.validateExportSQL(w, r, conn, input.SQL) {
		return
	}
	if !app.hasExportPermission(r, org.ID, ws.OwnerType, conn.ID) {
		app.notPermitted(w, r)
		return
	}
	job, err := app.workspaceJobStore().Enqueue(r.Context(), jobs.EnqueueInput{
		Type:           jobs.TypeExportQueryCSV,
		Visibility:     jobs.VisibilityUser,
		OrgID:          &org.ID,
		WorkspaceID:    &ws.ID,
		OwnerAccountID: &account.ID,
		Priority:       jobs.PriorityNormal,
		MaxAttempts:    1,
		Input: exportJobInput{
			AccountID:    account.ID,
			OrgID:        org.ID,
			WorkspaceID:  ws.ID,
			ConnectionID: conn.ID,
			SQL:          input.SQL,
			Format:       normalizedExportFormat(input.Format),
			Filename:     input.Filename,
		},
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "export job queued", slog.String("job.id", job.ID), slog.Int64("connection_id", conn.ID), slog.String("format", normalizedExportFormat(input.Format)))
	if err := response.JSON(w, http.StatusCreated, job); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) downloadConnectionExport(w http.ResponseWriter, r *http.Request) {
	input, ok := app.decodeExportRequest(w, r)
	if !ok {
		return
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	conn := contextGetConnection(r)
	if !app.validateExportSQL(w, r, conn, input.SQL) {
		return
	}
	if !app.hasExportPermission(r, org.ID, ws.OwnerType, conn.ID) {
		app.notPermitted(w, r)
		return
	}
	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return
	}
	session, found := app.connManager.Get(sessionID)
	if !found {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return
	}
	if session.AccountID != strconv.FormatInt(account.ID, 10) || session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return
	}

	filename := safeExportFilename(input.Filename, time.Now())
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.WriteHeader(http.StatusOK)
	result, err := exports.NewService().Stream(r.Context(), session.Conn, w, exports.StreamOptions{
		Format:   normalizedExportFormat(input.Format),
		SQL:      input.SQL,
		MaxBytes: app.config.Exports.SyncMaxBytes,
	})
	if err != nil {
		app.logWarn(r, "synchronous export failed", slog.Int64("connection_id", conn.ID), slog.String("session_id", sessionID), slog.String("error", exportErrorCategory(err)))
		if errors.Is(err, exports.ErrByteLimitExceeded) {
			panic(http.ErrAbortHandler)
		}
		return
	}
	app.logInfo(r, "synchronous export completed", slog.Int64("connection_id", conn.ID), slog.String("session_id", sessionID), slog.Int64("rows", result.Rows), slog.Int64("bytes", result.Bytes))
}

func (app *application) decodeExportRequest(w http.ResponseWriter, r *http.Request) (exportRequest, bool) {
	var input exportRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return exportRequest{}, false
	}
	input.SQL = strings.TrimSpace(input.SQL)
	input.Format = normalizedExportFormat(input.Format)
	input.Filename = strings.TrimSpace(input.Filename)
	input.V.CheckField(input.SQL != "", "sql", "SQL is required.")
	input.V.CheckField(input.Format == exports.FormatCSV, "format", "Format must be csv.")
	if input.Filename != "" {
		input.V.CheckField(files.ValidName(safeExportFilename(input.Filename, time.Now())), "filename", "Filename is invalid.")
	}
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return exportRequest{}, false
	}
	return input, true
}

func (app *application) validateExportSQL(w http.ResponseWriter, r *http.Request, conn database.Connection, sql string) bool {
	classification, err := app.classifyConnectionSQL(r, conn, sql)
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	if classification.Kind != classifier.KindDQL {
		app.failedValidation(w, r, fieldErrors(map[string]string{"sql": "Only read queries can be exported."}))
		return false
	}
	if classification.StatementCount > 1 {
		app.failedValidation(w, r, fieldErrors(map[string]string{"sql": "Only a single query can be exported. Multi-query export isn't supported yet."}))
		return false
	}
	return true
}

func (app *application) hasExportPermission(r *http.Request, orgID int64, ownerType string, connectionID int64) bool {
	return app.hasConnectionPermission(r, orgID, ownerType, connectionID, access.PermConnDQL) ||
		app.hasConnectionPermission(r, orgID, ownerType, connectionID, access.PermConnExecute)
}

func (app *application) handleExportJob(ctx context.Context, runtime jobs.Runtime) (any, error) {
	var input exportJobInput
	if err := json.Unmarshal([]byte(runtime.Job.InputJSON), &input); err != nil {
		return nil, jobs.Permanent("invalid_export_input", "Export job input is invalid.")
	}
	if input.Format == "" {
		input.Format = exports.FormatCSV
	}
	if input.Format != exports.FormatCSV {
		return nil, jobs.Permanent("unsupported_export_format", "Export format is not supported.")
	}
	runtime.Events.Info(ctx, "export_started", "Export started.", nil)

	org, found, err := app.db.GetOrg(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, jobs.Permanent("org_not_found", "Organization was not found.")
	}
	ws, found, err := app.db.GetWorkspace(ctx, input.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if !found || ws.OrgID == nil || *ws.OrgID != org.ID {
		return nil, jobs.Permanent("workspace_not_found", "Workspace was not found.")
	}
	conn, found, err := app.db.GetConnection(ctx, input.ConnectionID)
	if err != nil {
		return nil, err
	}
	if !found || conn.WorkspaceID != ws.ID {
		return nil, jobs.Permanent("connection_not_found", "Connection was not found.")
	}
	if !app.enforcer.Can(ctx, input.AccountID, org.ID, ws.OwnerType, "connection", conn.ID, access.PermConnDQL) &&
		!app.enforcer.Can(ctx, input.AccountID, org.ID, ws.OwnerType, "connection", conn.ID, access.PermConnExecute) {
		return nil, jobs.Permanent("export_not_permitted", "You no longer have permission to export this query.")
	}
	classification, err := connectionClassifier(conn.Driver).Classify(ctx, classifier.Request{SQL: input.SQL})
	if err != nil {
		return nil, err
	}
	if classification.Kind != classifier.KindDQL {
		return nil, jobs.Permanent("export_not_read_query", "Only read queries can be exported.")
	}
	if classification.StatementCount > 1 {
		return nil, jobs.Permanent("export_multi_statement", "Only a single query can be exported. Multi-query export isn't supported yet.")
	}

	plainDSN, err := app.keyring.Decrypt(conn.DSNEncrypted)
	if err != nil {
		return nil, err
	}
	if err := app.validateTargetConnection(conn.Driver, plainDSN); err != nil {
		return nil, jobs.Permanent("export_target_blocked", targetConnectionFieldError(err))
	}
	driver, err := dbengine.New(conn.Driver)
	if err != nil {
		return nil, err
	}
	if err := driver.Connect(ctx, dbengine.ConnectionConfig{DSN: plainDSN, Driver: conn.Driver}); err != nil {
		return nil, jobs.Retryable("export_connect_failed", "Could not connect to the target database.")
	}
	defer driver.Close()
	runtime.Events.Info(ctx, "target_connected", "Connected to database.", nil)

	scope := files.Scope{AccountID: input.AccountID, OrgID: org.ID, OrgSlug: org.Slug, Workspace: ws, Visibility: database.FileVisibilityPrivate}
	file, err := app.createExportFile(ctx, scope, input.Filename)
	if err != nil {
		return nil, err
	}
	runtime.Events.Info(ctx, "file_created", "Export file created.", map[string]any{"file_id": file.ID, "filename": file.Name})

	reader, writer := io.Pipe()
	var streamResult exports.StreamResult
	streamErr := make(chan error, 1)
	go func() {
		defer writer.Close()
		var lastProgress int64
		streamResult, err = exports.NewService().Stream(ctx, driver, writer, exports.StreamOptions{
			Format:   input.Format,
			SQL:      input.SQL,
			MaxBytes: app.config.Exports.BackgroundMaxBytes,
			OnProgress: func(rows int64, bytes int64) {
				if rows-lastProgress >= exportProgressEveryRows {
					lastProgress = rows
					runtime.Events.Info(ctx, "rows_streamed", "Rows exported.", map[string]any{"rows": rows, "bytes": bytes})
				}
			},
		})
		if err != nil {
			_ = writer.CloseWithError(err)
		}
		streamErr <- err
	}()
	_, writeErr := app.workspaceFileService().WriteContent(ctx, scope, file.ID, "", reader)
	if writeErr != nil {
		_ = reader.Close()
		if err := <-streamErr; err != nil {
			return nil, exportJobError(err)
		}
		return nil, writeErr
	}
	if err := <-streamErr; err != nil {
		return nil, exportJobError(err)
	}
	runtime.Events.Info(ctx, "export_succeeded", "Export completed.", map[string]any{"file_id": file.ID, "rows": streamResult.Rows, "bytes": streamResult.Bytes})
	return exportJobOutput{FileID: file.ID, Filename: file.Name, Format: input.Format, RowCount: streamResult.Rows, ByteCount: streamResult.Bytes}, nil
}

func (app *application) createExportFile(ctx context.Context, scope files.Scope, requested string) (database.WorkspaceFile, error) {
	service := app.workspaceFileService()
	folder, err := app.ensureExportsFolder(ctx, service, scope)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	name := safeExportFilename(requested, time.Now())
	for attempt := 0; attempt < 100; attempt++ {
		candidate := uniqueExportName(name, attempt)
		file, err := service.Create(ctx, scope, files.CreateInput{
			Name:      candidate,
			ParentID:  &folder.ID,
			MediaType: "text/csv; charset=utf-8",
			FileKind:  "export",
		})
		if err == nil {
			return file, nil
		}
		if !isUniqueViolation(err) {
			return database.WorkspaceFile{}, err
		}
	}
	return database.WorkspaceFile{}, fmt.Errorf("could not allocate export filename")
}

func (app *application) ensureExportsFolder(ctx context.Context, service *files.Service, scope files.Scope) (database.WorkspaceFile, error) {
	children, err := service.List(ctx, scope, nil)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	for _, child := range children {
		if child.ObjectType == database.FileObjectTypeFolder && strings.EqualFold(child.Name, "Exports") {
			return child, nil
		}
	}
	folder, err := service.Create(ctx, scope, files.CreateInput{Name: "Exports", ObjectType: database.FileObjectTypeFolder})
	if err == nil {
		return folder, nil
	}
	if isUniqueViolation(err) {
		children, listErr := service.List(ctx, scope, nil)
		if listErr != nil {
			return database.WorkspaceFile{}, listErr
		}
		for _, child := range children {
			if child.ObjectType == database.FileObjectTypeFolder && strings.EqualFold(child.Name, "Exports") {
				return child, nil
			}
		}
	}
	return database.WorkspaceFile{}, err
}

func normalizedExportFormat(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return exports.FormatCSV
	}
	return format
}

func safeExportFilename(name string, now time.Time) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "query-export-" + now.UTC().Format("20060102-150405")
	}
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.', r == ' ':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, " .-_")
	if name == "" {
		name = "query-export-" + now.UTC().Format("20060102-150405")
	}
	if len(name) > 80 {
		name = strings.TrimRight(name[:80], " .-_")
	}
	return name + ".csv"
}

func uniqueExportName(name string, attempt int) string {
	if attempt == 0 {
		return name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	return fmt.Sprintf("%s-%d%s", base, attempt+1, ext)
}

func exportJobError(err error) error {
	switch {
	case errors.Is(err, exports.ErrByteLimitExceeded):
		return jobs.Permanent("export_too_large", "Export exceeded the configured size limit.")
	case errors.Is(err, exports.ErrCursorUnsupported):
		return jobs.Permanent("export_unsupported_driver", "This database driver does not support streaming exports.")
	case errors.Is(err, context.Canceled):
		return err
	default:
		return jobs.Retryable("export_failed", "Export failed.")
	}
}

func exportErrorCategory(err error) string {
	switch {
	case errors.Is(err, exports.ErrByteLimitExceeded):
		return "byte_limit_exceeded"
	case errors.Is(err, exports.ErrCursorUnsupported):
		return "unsupported_driver"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "target_error"
	}
}
