package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/exportjob"
	"github.com/sqlwarden/internal/jobs"
	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

var errDirectoryStoreNotConfigured = errors.New("web: directory store is not configured")

type directoryStore interface {
	GetOrg(ctx context.Context, id int64) (database.Organization, bool, error)
	IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error)
	IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error)
	GetWorkspace(ctx context.Context, id int64) (database.Workspace, bool, error)
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
}

type directoryAdapter struct {
	store directoryStore
}

var _ access.Directory = directoryAdapter{}

func (d directoryAdapter) GetOrg(ctx context.Context, id int64) (access.Org, bool, error) {
	if d.store == nil {
		return access.Org{}, false, errDirectoryStoreNotConfigured
	}
	org, found, err := d.store.GetOrg(ctx, id)
	if err != nil || !found {
		return access.Org{}, found, err
	}
	return access.Org{ID: org.ID, Slug: org.Slug, Name: org.Name}, true, nil
}

func (d directoryAdapter) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	if d.store == nil {
		return false, errDirectoryStoreNotConfigured
	}
	return d.store.IsOrgMember(ctx, orgID, accountID)
}

func (d directoryAdapter) IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	if d.store == nil {
		return false, errDirectoryStoreNotConfigured
	}
	return d.store.IsEffectiveWorkspaceMember(ctx, orgID, workspaceID, accountID)
}

func (d directoryAdapter) GetWorkspace(ctx context.Context, id int64) (access.Workspace, bool, error) {
	if d.store == nil {
		return access.Workspace{}, false, errDirectoryStoreNotConfigured
	}
	workspace, found, err := d.store.GetWorkspace(ctx, id)
	if err != nil || !found {
		return access.Workspace{}, found, err
	}
	return access.Workspace{
		ID:        workspace.ID,
		OrgID:     workspace.OrgID,
		OwnerType: workspace.OwnerType,
		OwnerID:   workspace.OwnerID,
		Name:      workspace.Name,
	}, true, nil
}

func (d directoryAdapter) GetConnection(ctx context.Context, id int64) (access.Connection, bool, error) {
	if d.store == nil {
		return access.Connection{}, false, errDirectoryStoreNotConfigured
	}
	connection, found, err := d.store.GetConnection(ctx, id)
	if err != nil || !found {
		return access.Connection{}, found, err
	}
	return access.Connection{
		ID:            connection.ID,
		WorkspaceID:   connection.WorkspaceID,
		EnvironmentID: connection.EnvironmentID,
		Name:          connection.Name,
		Driver:        connection.Driver,
	}, true, nil
}

func (app *application) requirer() *access.Requirer {
	app.requirerOnce.Do(func() {
		app.accessRequirer = access.NewRequirer(directoryAdapter{store: app.db}, app.policy)
	})
	return app.accessRequirer
}

func (app *application) jobService() *jobs.Service {
	app.jobServiceOnce.Do(func() {
		app.jobSvc = jobs.NewService(app.workspaceJobStore(), app.requirer())
	})
	return app.jobSvc
}

func (app *application) workspaceJobStore() *jobs.DatabaseStore {
	if app.jobStore == nil {
		app.jobStore = jobs.NewStore(app.db)
	}
	return app.jobStore
}

func (app *application) serviceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, access.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, access.ErrForbidden):
		app.notPermitted(w, r)
	default:
		app.serverError(w, r, err)
	}
}

// queryService builds the query service on first use.
func (app *application) queryService() *queryapp.Service {
	app.queryOnce.Do(func() {
		app.querySvc = queryapp.New(queryapp.Deps{
			Store:    app.db,
			Settings: querySettings{app: app},
			Requirer: app.requirer(),
			Runtime:  app.runtime,
			Audit:    app.audit,
			Logger:   app.logger,
		})
	})
	return app.querySvc
}

type querySettings struct{ app *application }

func (s querySettings) ForWorkspace(ctx context.Context, ws access.Workspace) (queryapp.Policy, error) {
	effective, err := s.app.effectiveRuntimeSettingsForWorkspace(ctx, databaseWorkspace(ws))
	if err != nil {
		return queryapp.Policy{}, err
	}
	return queryapp.Policy{
		HistoryMode:      effective.QueryHistoryMode,
		HistoryRetention: effective.QueryHistoryRetentionCount,
		FavoritesMode:    effective.QueryFavoritesMode,
		MaxRows:          effective.QueryMaxResultRows,
		MaxBytes:         effective.QueryMaxResultBytes,
		CursorPageSize:   effective.QueryCursorPageSize,
	}, nil
}

// exportService builds the export service on first use.
func (app *application) exportService() *exportjob.Service {
	app.exportOnce.Do(func() {
		app.exportSvc = exportjob.New(exportjob.Deps{
			Requirer: app.requirer(),
			Queue:    app.workspaceJobStore(),
			Runtime:  app.runtime,
			Settings: exportSettings{app: app},
			Files:    exportFiles{app: app},
			Audit:    app.audit,
			Logger:   app.logger,
		})
	})
	return app.exportSvc
}

func databaseWorkspace(ws access.Workspace) database.Workspace {
	return database.Workspace{ID: ws.ID, OrgID: ws.OrgID, OwnerType: ws.OwnerType, OwnerID: ws.OwnerID, Name: ws.Name}
}

type exportSettings struct{ app *application }

func (s exportSettings) ForWorkspace(ctx context.Context, ws access.Workspace) (exportjob.Policy, error) {
	effective, err := s.app.effectiveRuntimeSettingsForWorkspace(ctx, databaseWorkspace(ws))
	if err != nil {
		return exportjob.Policy{}, err
	}
	return exportjob.Policy{
		SyncMaxBytes:       effective.ExportsSyncMaxBytes,
		BackgroundMaxBytes: effective.ExportsBackgroundMaxBytes,
	}, nil
}

type exportFiles struct{ app *application }

func (f exportFiles) ForWorkspace(ctx context.Context, ws access.Workspace) (exportjob.FileWriter, error) {
	effective, err := f.app.effectiveRuntimeSettingsForWorkspace(ctx, databaseWorkspace(ws))
	if err != nil {
		return nil, err
	}
	return f.app.workspaceFileServiceWithSettings(effective), nil
}

// queryError maps query service failures to the standard error envelope.
func (app *application) queryError(w http.ResponseWriter, r *http.Request, err error) {
	if !app.queryDomainError(w, r, err) {
		app.serverError(w, r, err)
	}
}

// queryRuntimeError maps query service failures from operations that reach
// the execution runtime; anything that is not a domain failure is a runtime
// failure.
func (app *application) queryRuntimeError(w http.ResponseWriter, r *http.Request, err error) {
	if !app.queryDomainError(w, r, err) {
		app.executionError(w, r, err)
	}
}

func (app *application) queryDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	var validation *queryapp.ValidationError
	var disabled *queryapp.DisabledError
	var unsafe *queryapp.UnsafeError
	switch {
	case errors.As(err, &validation):
		v := validator.Validator{}
		for field, message := range validation.Fields {
			v.AddFieldError(field, message)
		}
		app.failedValidation(w, r, v)
	case errors.As(err, &disabled):
		v := validator.Validator{}
		v.AddError(disabled.Message)
		app.failedValidation(w, r, v)
	case errors.As(err, &unsafe):
		app.apiError(w, r, http.StatusUnprocessableEntity,
			"unsafe_query_confirmation_required",
			"This statement has no WHERE clause and will affect every row. Confirm to run it anyway.",
			response.APIError{Details: unsafe.Statements},
			nil,
		)
	case errors.Is(err, queryapp.ErrSessionRequired):
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
	case errors.Is(err, queryapp.ErrExplainUnsupported):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "This connection does not support EXPLAIN.", nil)
	case errors.Is(err, queryapp.ErrExplainAnalyzeUnsupported):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "This connection does not support EXPLAIN ANALYZE.", nil)
	case errors.Is(err, queryapp.ErrExplainMultiple):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "EXPLAIN requires exactly one statement.", nil)
	case errors.Is(err, queryapp.ErrExplainAlready):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "This statement is already an EXPLAIN statement.", nil)
	case errors.Is(err, access.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, access.ErrForbidden):
		app.notPermitted(w, r)
	default:
		return false
	}
	return true
}
