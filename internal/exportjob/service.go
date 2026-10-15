package exportjob

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/query"
)

const actionExportRequested = "export.requested"

// exportPermissions are the connection permissions that each allow exporting a
// read query.
var exportPermissions = []string{access.PermConnDQL, access.PermConnExecute}

// Policy is the effective export configuration for a workspace.
type Policy struct {
	SyncMaxBytes       int64
	BackgroundMaxBytes int64
}

// Settings resolves the effective Policy for a workspace.
type Settings interface {
	ForWorkspace(ctx context.Context, ws access.Workspace) (Policy, error)
}

// Runtime is the subset of the execution runtime exports drive.
type Runtime interface {
	Open(ctx context.Context, req execution.OpenRequest) (execution.SessionInfo, error)
	Get(ctx context.Context, scope execution.Scope, id execution.SessionID) (execution.SessionInfo, error)
	Close(ctx context.Context, scope execution.Scope, id execution.SessionID) error
	Stream(ctx context.Context, scope execution.Scope, id execution.SessionID, req execution.StreamRequest, w io.Writer) (exports.StreamResult, error)
}

// FileWriter is the workspace-file surface an export file is written through.
type FileWriter interface {
	List(ctx context.Context, p access.Principal, req files.Scope, parentID *int64) ([]database.WorkspaceFile, error)
	Create(ctx context.Context, p access.Principal, req files.Scope, input files.CreateInput) (database.WorkspaceFile, error)
	WriteContent(ctx context.Context, p access.Principal, req files.Scope, fileID int64, expectedHash string, content io.Reader) (database.WorkspaceFileContent, error)
}

// Files resolves the file service that applies a workspace's file policy.
type Files interface {
	ForWorkspace(ctx context.Context, ws access.Workspace) (FileWriter, error)
}

// Queue persists background jobs.
type Queue interface {
	Enqueue(ctx context.Context, input jobs.EnqueueInput) (jobs.Record, error)
}

// ClassifierLookup resolves the classifier a driver registered itself.
type ClassifierLookup func(driver string) (classifier.Classifier, bool)

type Deps struct {
	Requirer *access.Requirer
	Queue    Queue
	Runtime  Runtime
	Settings Settings
	Files    Files
	// Classifier defaults to query.RegisteredClassifier.
	Classifier ClassifierLookup
	Audit      audit.Writer
	Logger     *slog.Logger
	Now        func() time.Time
}

type Service struct {
	deps Deps
}

func New(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if deps.Classifier == nil {
		deps.Classifier = query.RegisteredClassifier
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Service{deps: deps}
}

// RequestInput is the caller-supplied export definition.
type RequestInput struct {
	SQL      string
	Format   string
	Filename string
}

// JobInput is the persisted input of an export job.
type JobInput struct {
	AccountID    int64  `json:"account_id"`
	OrgID        int64  `json:"org_id"`
	WorkspaceID  int64  `json:"workspace_id"`
	ConnectionID int64  `json:"connection_id"`
	SQL          string `json:"sql"`
	Format       string `json:"format"`
	Filename     string `json:"filename,omitempty"`
}

// JobOutput is the persisted result of a successful export job.
type JobOutput struct {
	FileID    int64  `json:"file_id"`
	Filename  string `json:"filename"`
	Format    string `json:"format"`
	RowCount  int64  `json:"row_count"`
	ByteCount int64  `json:"byte_count"`
}

// normalize trims and defaults the input, then validates its shape.
func (s *Service) normalize(in RequestInput) (RequestInput, error) {
	in.SQL = strings.TrimSpace(in.SQL)
	in.Format = NormalizedFormat(in.Format)
	in.Filename = strings.TrimSpace(in.Filename)
	fields := map[string]string{}
	if in.SQL == "" {
		fields["sql"] = "SQL is required."
	}
	if in.Format != exports.FormatCSV {
		fields["format"] = "Format must be csv."
	}
	if in.Filename != "" && !files.ValidName(safeFilename(in.Filename, s.deps.Now())) {
		fields["filename"] = "Filename is invalid."
	}
	if len(fields) > 0 {
		return RequestInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

// classify proves sql is a single read query for the driver.
func (s *Service) classify(ctx context.Context, driver, sql string) error {
	c, ok := s.deps.Classifier(driver)
	if !ok {
		return ErrClassifierUnavailable
	}
	result, err := c.Classify(ctx, classifier.Request{SQL: sql})
	if err != nil {
		return err
	}
	if result.Kind != classifier.KindDQL {
		return errNotReadQuery
	}
	if result.StatementCount != 1 {
		return errMultiStatement
	}
	return nil
}

// prepare authorizes the principal on the connection and validates the input.
func (s *Service) prepare(ctx context.Context, p access.Principal, ref access.ConnRef, in RequestInput) (access.Connection, access.Workspace, RequestInput, error) {
	conn, ws, err := s.deps.Requirer.Connection(ctx, p, ref, exportPermissions...)
	if err != nil {
		return access.Connection{}, access.Workspace{}, RequestInput{}, err
	}
	in, err = s.normalize(in)
	if err != nil {
		return access.Connection{}, access.Workspace{}, RequestInput{}, err
	}
	switch err := s.classify(ctx, conn.Driver, in.SQL); {
	case errors.Is(err, errNotReadQuery):
		return access.Connection{}, access.Workspace{}, RequestInput{}, validationError("sql", messageNotReadQuery)
	case errors.Is(err, errMultiStatement):
		return access.Connection{}, access.Workspace{}, RequestInput{}, validationError("sql", messageMultiStatement)
	case err != nil:
		return access.Connection{}, access.Workspace{}, RequestInput{}, err
	}
	return conn, ws, in, nil
}

// Request validates the export and queues it as a user-visible background job
// owned by the principal.
func (s *Service) Request(ctx context.Context, p access.Principal, ref access.ConnRef, in RequestInput) (jobs.Record, error) {
	conn, ws, in, err := s.prepare(ctx, p, ref, in)
	if err != nil {
		return jobs.Record{}, err
	}
	orgID, workspaceID, accountID := ref.OrgID, ws.ID, p.Subject.ID
	job, err := s.deps.Queue.Enqueue(ctx, jobs.EnqueueInput{
		Type:           jobs.TypeExportQueryCSV,
		Visibility:     jobs.VisibilityUser,
		OrgID:          &orgID,
		WorkspaceID:    &workspaceID,
		OwnerAccountID: &accountID,
		Priority:       jobs.PriorityNormal,
		MaxAttempts:    1,
		Input: JobInput{
			AccountID:    accountID,
			OrgID:        orgID,
			WorkspaceID:  workspaceID,
			ConnectionID: conn.ID,
			SQL:          in.SQL,
			Format:       in.Format,
			Filename:     in.Filename,
		},
	})
	if err != nil {
		return jobs.Record{}, err
	}
	s.emitRequested(ctx, p, orgID, conn.ID, "queued", map[string]string{"job_id": job.ID})
	return job, nil
}

// Download streams the export into w through the principal's live session id;
// an empty id yields ErrSessionRequired. onStart receives the
// download filename after authorization and session ownership succeed and
// before the first byte is written, so a caller sets response headers only for
// requests that will stream. Errors returned before onStart runs have written
// nothing.
func (s *Service) Download(ctx context.Context, p access.Principal, ref access.ConnRef, id execution.SessionID, in RequestInput, w io.Writer, onStart func(filename string)) (exports.StreamResult, error) {
	conn, ws, in, err := s.prepare(ctx, p, ref, in)
	if err != nil {
		return exports.StreamResult{}, err
	}
	if id == "" {
		return exports.StreamResult{}, ErrSessionRequired
	}
	scope := execScope(ref.OrgID, ws.ID, p.Subject.ID, conn.ID)
	if _, err := s.deps.Runtime.Get(ctx, scope, id); err != nil {
		return exports.StreamResult{}, err
	}
	policy, err := s.deps.Settings.ForWorkspace(ctx, ws)
	if err != nil {
		return exports.StreamResult{}, err
	}

	filename := safeFilename(in.Filename, s.deps.Now())
	s.emitRequested(ctx, p, ref.OrgID, conn.ID, "download", nil)
	if onStart != nil {
		onStart(filename)
	}
	result, err := s.deps.Runtime.Stream(ctx, scope, id, execution.StreamRequest{
		Format: in.Format,
		SQL:    in.SQL,
		Limits: execution.Limits{MaxBytes: policy.SyncMaxBytes},
	}, w)
	if errors.Is(err, exports.ErrByteLimitExceeded) {
		s.deps.Logger.DebugContext(ctx, "export download aborted at byte limit",
			slog.Int64("connection_id", conn.ID), slog.Int64("max_bytes", policy.SyncMaxBytes),
			slog.Int64("rows", result.Rows), slog.Int64("bytes", result.Bytes))
	}
	return result, err
}

func execScope(orgID, workspaceID, accountID, connectionID int64) execution.Scope {
	return execution.Scope{
		OrgID:        strconv.FormatInt(orgID, 10),
		WorkspaceID:  strconv.FormatInt(workspaceID, 10),
		AccountID:    strconv.FormatInt(accountID, 10),
		ConnectionID: strconv.FormatInt(connectionID, 10),
	}
}

// emitRequested records the export request. Audit failures never fail the
// export: they are logged and the export proceeds.
func (s *Service) emitRequested(ctx context.Context, p access.Principal, orgID, connectionID int64, via string, extra map[string]string) {
	metadata := map[string]string{"via": via}
	for key, value := range extra {
		metadata[key] = value
	}
	err := audit.Emit(ctx, s.deps.Audit, audit.Event{
		OrgID:      &orgID,
		Actor:      audit.ActorFromPrincipal(p),
		Action:     actionExportRequested,
		Resource:   "connection",
		ResourceID: strconv.FormatInt(connectionID, 10),
		Outcome:    audit.OutcomeSuccess,
		Metadata:   metadata,
	})
	if err != nil {
		s.deps.Logger.WarnContext(ctx, "export audit event failed", slog.Int64("connection_id", connectionID), slog.Any("error", err))
	}
}
