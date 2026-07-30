package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type schemaSpecResponse struct {
	Spec metadata.SchemaSpec `json:"spec"`
}

type directoryResponse struct {
	Directory *metadata.Directory `json:"directory"`
}

type objectsRequest struct {
	Refs []metadata.ObjectRef `json:"refs"`
}

type objectsResponse struct {
	Objects []metadata.Object `json:"objects"`
}

type refreshRequest struct {
	Ref *metadata.ObjectRef `json:"ref"`
}

type relationshipsResponse struct {
	Graph *metadata.RelationshipGraph `json:"graph"`
}

type schemaStatusResponse struct {
	Status      string     `json:"status"`
	Mode        string     `json:"mode"`
	SnapshotID  string     `json:"snapshot_id,omitempty"`
	GeneratedAt *time.Time `json:"generated_at,omitempty"`
	Stale       bool       `json:"stale,omitempty"`
	JobID       string     `json:"job_id,omitempty"`
}

func (app *application) authorizeSchemaAccess(w http.ResponseWriter, r *http.Request) bool {
	org := contextGetOrg(r)
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)
	if app.hasAnyConnectionRuntimePermission(r, org.ID, ws.OwnerType, conn.ID,
		access.PermConnExecute, access.PermConnDQL, access.PermConnDML, access.PermConnDDL) {
		return true
	}
	app.notPermitted(w, r)
	return false
}

func (app *application) persistentSchemaMode(r *http.Request) (bool, error) {
	return app.db.SchemaSnapshotsEnabled(r.Context(), contextGetConnection(r).ID)
}

func (app *application) writeSnapshotPending(w http.ResponseWriter, r *http.Request) {
	conn := contextGetConnection(r)
	status := schemaStatusResponse{Status: "pending", Mode: "persistent"}
	if job, found, err := app.workspaceJobStore().ActiveBySingletonKey(r.Context(), schemaSyncSingletonKey(conn.ID)); err == nil && found {
		status.JobID = job.ID
	}
	if err := response.JSON(w, http.StatusAccepted, status); err != nil {
		app.serverError(w, r, err)
	}
}

// resolveSchemaSession applies the same preconditions as executeQuery: a valid
// X-Warden-Session header, the session belonging to the caller and connection,
// and any-runtime-permission on the connection. It writes the error response
// and returns ok=false on failure.
func (app *application) resolveSchemaSession(w http.ResponseWriter, r *http.Request) (*connection.Session, bool) {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)

	if !app.hasAnyConnectionRuntimePermission(r, org.ID, ws.OwnerType, conn.ID,
		access.PermConnExecute, access.PermConnDQL, access.PermConnDML, access.PermConnDDL) {
		app.logWarn(r, "schema access denied",
			slog.Int64("connection_id", conn.ID),
			slog.Int64("workspace_id", ws.ID),
			slog.String("reason", "missing_runtime_permission"),
		)
		app.notPermitted(w, r)
		return nil, false
	}

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.logWarn(r, "schema session missing", slog.Int64("connection_id", conn.ID))
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return nil, false
	}
	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.logWarn(r, "schema session unavailable",
			slog.String("session_id", sessionID),
			slog.Int64("connection_id", conn.ID),
		)
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return nil, false
	}
	if session.AccountID != strconv.FormatInt(account.ID, 10) ||
		session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.logWarn(r, "schema session scope mismatch",
			slog.String("session_id", session.ID),
			slog.String("session_account_id", session.AccountID),
			slog.String("session_connection_id", session.ConnectionID),
			slog.Int64("account_id", account.ID),
			slog.Int64("connection_id", conn.ID),
		)
		app.notPermitted(w, r)
		return nil, false
	}
	return session, true
}

// resolveSchemaInspector resolves the active database session and checks whether
// the concrete driver supports schema inspection.
func (app *application) resolveSchemaInspector(w http.ResponseWriter, r *http.Request) (*connection.Session, metadata.SchemaInspector, bool) {
	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return nil, nil, false
	}
	inspector, ok := session.Conn.(metadata.SchemaInspector)
	if !ok {
		app.logWarn(r, "schema inspection unsupported",
			slog.String("session_id", session.ID),
			slog.Int64("connection_id", contextGetConnection(r).ID),
		)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema inspection.", nil)
		return nil, nil, false
	}
	return session, inspector, true
}

// resolveRelationshipInspector resolves the session and asserts the optional
// relationship capability, returning 501 when the driver lacks it.
func (app *application) resolveRelationshipInspector(w http.ResponseWriter, r *http.Request) (*connection.Session, metadata.RelationshipInspector, bool) {
	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return nil, nil, false
	}
	inspector, ok := session.Conn.(metadata.RelationshipInspector)
	if !ok {
		app.logWarn(r, "schema relationships unsupported",
			slog.String("session_id", session.ID),
			slog.Int64("connection_id", contextGetConnection(r).ID),
		)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema relationships.", nil)
		return nil, nil, false
	}
	return session, inspector, true
}

func (app *application) getConnectionSchemaRelationships(w http.ResponseWriter, r *http.Request) {
	scope, err := schemaScopeQuery(r)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}
	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if persistent {
		if !app.authorizeSchemaAccess(w, r) {
			return
		}
		snapshot, _, found, err := app.schemaSnapshots.Active(r.Context(), contextGetConnection(r).ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.writeSnapshotPending(w, r)
			return
		}
		graph, found, err := app.schemaSnapshots.Relationship(r.Context(), snapshot.ID, scope)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema relationships.", nil)
			return
		}
		if err := response.JSON(w, http.StatusOK, relationshipsResponse{Graph: graph}); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	session, inspector, ok := app.resolveRelationshipInspector(w, r)
	if !ok {
		return
	}
	graph, err := app.schemaService.Relationships(r.Context(), session.ConnectionID, scope, inspector)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logDebug(r, "schema relationships returned",
		slog.String("session_id", session.ID),
		slog.String("scope", string(scope)),
		slog.Int("edge_count", len(graph.Relationships)),
	)
	if err := response.JSON(w, http.StatusOK, relationshipsResponse{Graph: graph}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaSpec(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	driver, err := engine.New(contextGetConnection(r).Driver)
	if err != nil {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema inspection.", nil)
		return
	}
	inspector, ok := driver.(metadata.SchemaInspector)
	if !ok {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema inspection.", nil)
		return
	}
	spec := app.schemaService.Spec(inspector)
	app.logDebug(r, "schema spec returned",
		slog.String("dialect", spec.Dialect),
		slog.Int("kind_count", len(spec.Kinds)),
	)
	if err := response.JSON(w, http.StatusOK, schemaSpecResponse{Spec: spec}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaDirectory(w http.ResponseWriter, r *http.Request) {
	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if persistent {
		if !app.authorizeSchemaAccess(w, r) {
			return
		}
		_, directory, found, err := app.schemaSnapshots.Active(r.Context(), contextGetConnection(r).ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.writeSnapshotPending(w, r)
			return
		}
		if err := response.JSON(w, http.StatusOK, directoryResponse{Directory: directory}); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	session, inspector, ok := app.resolveSchemaInspector(w, r)
	if !ok {
		return
	}
	directory, err := app.schemaService.Directory(r.Context(), session.ConnectionID, inspector)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logDebug(r, "schema directory returned",
		slog.String("session_id", session.ID),
		slog.String("engine", directory.Engine),
	)
	if err := response.JSON(w, http.StatusOK, directoryResponse{Directory: directory}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjects(w http.ResponseWriter, r *http.Request) {
	var input objectsRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if persistent {
		if !app.authorizeSchemaAccess(w, r) {
			return
		}
		snapshot, _, found, err := app.schemaSnapshots.Active(r.Context(), contextGetConnection(r).ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if !found {
			app.writeSnapshotPending(w, r)
			return
		}
		objects, err := app.schemaSnapshots.Objects(r.Context(), snapshot.ID, input.Refs)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if err := response.JSON(w, http.StatusOK, objectsResponse{Objects: objects}); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	session, inspector, ok := app.resolveSchemaInspector(w, r)
	if !ok {
		return
	}
	objects, err := app.schemaService.Objects(r.Context(), session.ConnectionID, input.Refs, inspector)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logDebug(r, "schema objects returned",
		slog.String("session_id", session.ID),
		slog.Int("requested_ref_count", len(input.Refs)),
		slog.Int("object_count", len(objects)),
	)
	if err := response.JSON(w, http.StatusOK, objectsResponse{Objects: objects}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshConnectionSchema(w http.ResponseWriter, r *http.Request) {
	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if persistent {
		if !app.authorizeSchemaAccess(w, r) {
			return
		}
		conn := contextGetConnection(r)
		ws := contextGetWorkspace(r)
		job, created, err := app.enqueueSchemaSync(r.Context(), conn.ID, ws.OrgID)
		if err != nil && !errors.Is(err, jobs.ErrActiveExists) {
			app.serverError(w, r, err)
			return
		}
		if !created {
			if active, found, lookupErr := app.workspaceJobStore().ActiveBySingletonKey(r.Context(), schemaSyncSingletonKey(conn.ID)); lookupErr == nil && found {
				job = active
			}
		}
		if err := response.JSON(w, http.StatusAccepted, schemaStatusResponse{
			Status: "pending", Mode: "persistent", JobID: job.ID,
		}); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	session, _, ok := app.resolveSchemaInspector(w, r)
	if !ok {
		return
	}
	var input refreshRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := request.DecodeJSON(w, r, &input); err != nil {
			app.badRequest(w, r, err)
			return
		}
	}
	if input.Ref != nil {
		app.schemaService.RefreshObject(session.ConnectionID, *input.Ref)
		app.completionService.InvalidateConnection(session.ConnectionID)
		app.logInfo(r, "schema object cache refresh requested",
			slog.String("session_id", session.ID),
			slog.String("connection_id", session.ConnectionID),
			slog.String("kind", input.Ref.Kind),
			slog.String("scope", string(input.Ref.Scope)),
			slog.String("name", input.Ref.Name),
		)
	} else {
		app.schemaService.RefreshConnection(session.ConnectionID)
		app.completionService.InvalidateConnection(session.ConnectionID)
		app.logInfo(r, "schema connection cache refresh requested",
			slog.String("session_id", session.ID),
			slog.String("connection_id", session.ConnectionID),
		)
	}
	if err := response.JSON(w, http.StatusOK, schemaStatusResponse{Status: "ok", Mode: "ephemeral"}); err != nil {
		app.serverError(w, r, err)
	}
}

func schemaScopeQuery(r *http.Request) (metadata.ScopePath, error) {
	raw := r.URL.Query().Get("scope")
	if raw == "" {
		return "", errors.New("scope query parameter is required")
	}
	var scope metadata.ScopePath
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		return "", err
	}
	if scope == "" {
		return "", errors.New("scope query parameter must not be empty")
	}
	return scope, nil
}

func (app *application) getConnectionSchemaSnapshot(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	conn := contextGetConnection(r)
	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !persistent {
		if err := response.JSON(w, http.StatusOK, schemaStatusResponse{Status: "available", Mode: "ephemeral"}); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	snapshot, _, found, err := app.schemaSnapshots.Active(r.Context(), conn.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.writeSnapshotPending(w, r)
		return
	}
	status := schemaStatusResponse{
		Status: "available", Mode: "persistent", SnapshotID: snapshot.ID,
		GeneratedAt: &snapshot.GeneratedAt,
		Stale:       time.Since(snapshot.GeneratedAt) >= app.config.Schema.SnapshotFreshness,
	}
	if job, active, lookupErr := app.workspaceJobStore().ActiveBySingletonKey(r.Context(), schemaSyncSingletonKey(conn.ID)); lookupErr == nil && active {
		status.Status = "refreshing"
		status.JobID = job.ID
	}
	if err := response.JSON(w, http.StatusOK, status); err != nil {
		app.serverError(w, r, err)
	}
}
