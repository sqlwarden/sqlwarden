package exportjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/jobs"
	"github.com/uptrace/bun/driver/pgdriver"
)

const progressEveryRows int64 = 10000

// jobPrincipal is the owner of a queued export acting through the job runner.
// It carries no session, so every grant is re-evaluated against current policy.
func jobPrincipal(accountID int64) access.Principal {
	return access.Principal{
		Subject:    access.SubjectRef{Kind: access.SubjectAccount, ID: accountID},
		Credential: access.CredentialInfo{Kind: access.CredentialJob},
	}
}

// authorizeJob re-resolves the export's resources as the job owner. Resources
// that vanished or moved fail permanently, as does lost access.
func (s *Service) authorizeJob(ctx context.Context, p access.Principal, input JobInput) (access.Connection, access.Workspace, error) {
	notPermitted := jobs.Permanent("export_not_permitted", "You no longer have permission to export this query.")
	translate := func(err error, notFound error) error {
		switch {
		case errors.Is(err, access.ErrForbidden):
			return notPermitted
		case errors.Is(err, access.ErrNotFound):
			return notFound
		default:
			return err
		}
	}
	conn, ws, err := s.deps.Requirer.Connection(ctx, p, access.ConnRef{
		OrgID: input.OrgID, WorkspaceID: input.WorkspaceID, ConnectionID: input.ConnectionID,
	}, exportPermissions...)
	if err != nil {
		return access.Connection{}, access.Workspace{}, translate(err, jobs.Permanent("connection_not_found", "Connection was not found."))
	}
	return conn, ws, nil
}

// RunJob executes a queued export: it re-authorizes the owner, re-classifies
// the SQL, streams the result through an ephemeral session into an export file.
func (s *Service) RunJob(ctx context.Context, runtime jobs.Runtime) (any, error) {
	var input JobInput
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

	principal := jobPrincipal(input.AccountID)
	conn, ws, err := s.authorizeJob(ctx, principal, input)
	if err != nil {
		return nil, err
	}
	switch err := s.classify(ctx, conn.Driver, input.SQL); {
	case errors.Is(err, ErrClassifierUnavailable):
		return nil, jobs.Permanent("export_classifier_unavailable", messageClassifierUnavailable)
	case errors.Is(err, errNotReadQuery):
		return nil, jobs.Permanent("export_not_read_query", messageNotReadQuery)
	case errors.Is(err, errMultiStatement):
		return nil, jobs.Permanent("export_multi_statement", messageMultiStatement)
	case err != nil:
		return nil, err
	}

	scope := execScope(input.OrgID, ws.ID, input.AccountID, conn.ID)
	session, err := s.deps.Runtime.Open(ctx, execution.OpenRequest{Scope: scope, Ephemeral: true})
	if err != nil {
		return nil, openError(err)
	}
	defer func() {
		if err := s.deps.Runtime.Close(context.WithoutCancel(ctx), scope, session.ID); err != nil {
			s.deps.Logger.WarnContext(ctx, "export session close failed",
				slog.Int64("connection_id", conn.ID), slog.String("session_id", string(session.ID)), slog.Any("error", err))
		}
	}()
	runtime.Events.Info(ctx, "target_connected", "Connected to database.", nil)
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return nil, err
	}
	fileService, err := s.deps.Files.ForWorkspace(ctx, ws)
	if err != nil {
		return nil, err
	}

	fileScope := files.Scope{OrgID: input.OrgID, WorkspaceID: ws.ID, Visibility: database.FileVisibilityPrivate}
	file, err := s.createFile(ctx, fileService, principal, fileScope, input.Filename)
	if err != nil {
		return nil, err
	}
	runtime.Events.Info(ctx, "file_created", "Export file created.", map[string]any{"file_id": file.ID, "filename": file.Name})

	reader, writer := io.Pipe()
	defer reader.Close()
	var streamResult exports.StreamResult
	streamErr := make(chan error, 1)
	go func() {
		defer writer.Close()
		var lastProgress int64
		var err error
		streamResult, err = s.deps.Runtime.Stream(ctx, scope, session.ID, execution.StreamRequest{
			Format: input.Format,
			SQL:    input.SQL,
			Limits: execution.Limits{MaxBytes: policy.BackgroundMaxBytes},
			OnProgress: func(rows int64, bytes int64) {
				if rows-lastProgress >= progressEveryRows {
					lastProgress = rows
					runtime.Events.Info(ctx, "rows_streamed", "Rows exported.", map[string]any{"rows": rows, "bytes": bytes})
				}
			},
		}, writer)
		if err != nil {
			_ = writer.CloseWithError(err)
		}
		streamErr <- err
	}()
	_, writeErr := fileService.WriteContent(ctx, principal, fileScope, file.ID, "", reader)
	if writeErr != nil {
		_ = reader.Close()
		// The stream fails with a closed-pipe error only because the write
		// side was abandoned; the write failure is the root cause.
		if err := <-streamErr; err != nil {
			if !errors.Is(err, io.ErrClosedPipe) {
				s.logStreamAbort(ctx, runtime.Job.ID, conn.ID, policy.BackgroundMaxBytes, err)
				return nil, streamError(err)
			}
			s.deps.Logger.DebugContext(ctx, "export stream stopped after the file write ended early",
				slog.String("job_id", runtime.Job.ID), slog.Int64("connection_id", conn.ID))
		}
		return nil, writeErr
	}
	if err := <-streamErr; err != nil {
		s.logStreamAbort(ctx, runtime.Job.ID, conn.ID, policy.BackgroundMaxBytes, err)
		return nil, streamError(err)
	}
	runtime.Events.Info(ctx, "export_succeeded", "Export completed.", map[string]any{"file_id": file.ID, "rows": streamResult.Rows, "bytes": streamResult.Bytes})
	return JobOutput{FileID: file.ID, Filename: file.Name, Format: input.Format, RowCount: streamResult.Rows, ByteCount: streamResult.Bytes}, nil
}

func (s *Service) logStreamAbort(ctx context.Context, jobID string, connectionID, maxBytes int64, err error) {
	if errors.Is(err, exports.ErrByteLimitExceeded) {
		s.deps.Logger.DebugContext(ctx, "export job aborted at byte limit",
			slog.String("job_id", jobID), slog.Int64("connection_id", connectionID), slog.Int64("max_bytes", maxBytes))
	}
}

func (s *Service) createFile(ctx context.Context, service FileWriter, principal access.Principal, scope files.Scope, requested string) (database.WorkspaceFile, error) {
	folder, err := ensureExportsFolder(ctx, principal, service, scope)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	name := safeFilename(requested, s.deps.Now())
	for attempt := 0; attempt < 100; attempt++ {
		file, err := service.Create(ctx, principal, scope, files.CreateInput{
			Name:      uniqueName(name, attempt),
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

func findExportsFolder(children []database.WorkspaceFile) (database.WorkspaceFile, bool) {
	for _, child := range children {
		if child.ObjectType == database.FileObjectTypeFolder && strings.EqualFold(child.Name, "Exports") {
			return child, true
		}
	}
	return database.WorkspaceFile{}, false
}

func ensureExportsFolder(ctx context.Context, principal access.Principal, service FileWriter, scope files.Scope) (database.WorkspaceFile, error) {
	children, err := service.List(ctx, principal, scope, nil)
	if err != nil {
		return database.WorkspaceFile{}, err
	}
	if folder, ok := findExportsFolder(children); ok {
		return folder, nil
	}
	folder, err := service.Create(ctx, principal, scope, files.CreateInput{Name: "Exports", ObjectType: database.FileObjectTypeFolder})
	if err == nil {
		return folder, nil
	}
	if isUniqueViolation(err) {
		children, listErr := service.List(ctx, principal, scope, nil)
		if listErr != nil {
			return database.WorkspaceFile{}, listErr
		}
		if folder, ok := findExportsFolder(children); ok {
			return folder, nil
		}
	}
	return database.WorkspaceFile{}, err
}

// isUniqueViolation reports a unique-constraint failure from either the
// PostgreSQL or SQLite driver.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
