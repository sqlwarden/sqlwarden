package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type objectsRequest struct {
	Refs []metadata.ObjectRef `json:"refs"`
}

type objectsResponse struct {
	Objects []metadata.Object `json:"objects"`
}

type objectDefinitionResponse struct {
	Descriptor *metadata.Descriptor `json:"descriptor"`
}

type relationshipsResponse struct {
	Graph *metadata.RelationshipGraph `json:"graph"`
}

type generateStatementRequest struct {
	Operation statement.Operation `json:"operation"`
	Ref       metadata.ObjectRef  `json:"ref"`
}

type generateStatementResponse struct {
	SQL string `json:"sql"`
}

type schemaStatusResponse struct {
	Status string `json:"status"`
	Mode   string `json:"mode"`
	Stale  bool   `json:"stale,omitempty"`
}

type schemaEditResponse struct {
	Applied     bool                   `json:"applied"`
	Schema      schemaStatusResponse   `json:"schema"`
	Transaction transactionStatusView  `json:"transaction"`
	Listings    []navigatorListingView `json:"listings,omitempty"`
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

// schemaSession is a live session together with what its driver supports, so
// handlers decide capability support without touching the driver.
type schemaSession struct {
	id   execution.SessionID
	caps execution.SessionCapabilities
}

// resolveSchemaSession applies the same preconditions as executeQuery: a valid
// X-Warden-Session header, the session belonging to the request scope, and
// any-runtime-permission on the connection. It writes the error response and
// returns ok=false on failure.
func (app *application) resolveSchemaSession(w http.ResponseWriter, r *http.Request) (*schemaSession, bool) {
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

	sessionID := requestSessionID(r)
	if sessionID == "" {
		app.logWarn(r, "schema session missing", slog.Int64("connection_id", conn.ID))
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return nil, false
	}
	caps, err := app.runtime.Capabilities(r.Context(), runtimeScope(r), sessionID)
	if err != nil {
		app.logWarn(r, "schema session unavailable",
			slog.String("session_id", string(sessionID)),
			slog.Int64("connection_id", conn.ID),
		)
		app.executionError(w, r, err)
		return nil, false
	}
	return &schemaSession{id: sessionID, caps: caps}, true
}

func (app *application) getConnectionSchemaRelationships(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	scope, err := schemaScopeQuery(r)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	if session != nil && !session.caps.Relationships {
		app.logInfo(r, "schema relationships unsupported", slog.Int64("connection_id", contextGetConnection(r).ID))
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema relationships.", nil)
		return
	}
	live := app.sessionLive(r, session)
	conn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	graph, err := app.schemaNavigator.Relationships(r.Context(), conn, live, scope)
	if err != nil {
		app.navigatorError(w, r, err)
		return
	}
	app.logDebug(r, "schema relationships returned",
		slog.Int64("connection_id", conn.ID),
		slog.Int("edge_count", len(graph.Relationships)),
		slog.Bool("session", session != nil),
	)
	if err := response.JSON(w, http.StatusOK, relationshipsResponse{Graph: graph}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) generateConnectionStatement(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	var input generateStatementRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	generator, ok := engine.StatementGenerator(contextGetConnection(r).Driver)
	if !ok {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support statement generation.", nil)
		return
	}
	if !generator.StatementSpec().Supports(input.Ref.Kind, input.Operation) {
		app.apiError(w, r, http.StatusUnprocessableEntity, "statement_generation_failed", "The requested statement is not supported for this object.", response.APIError{}, nil)
		return
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	if !app.sessionInspectsSchema(w, r, session) {
		return
	}
	navConn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	objects, err := app.schemaNavigator.Objects(r.Context(), navConn, app.sessionLive(r, session), []metadata.ObjectRef{input.Ref})
	if err != nil {
		app.navigatorError(w, r, err)
		return
	}
	if len(objects) != 1 || objects[0].Ref != input.Ref {
		app.errorMessage(w, r, http.StatusNotFound, "Schema object was not found.", nil)
		return
	}
	generated, err := generator.Generate(statement.Request{Operation: input.Operation, Object: objects[0]})
	if err != nil {
		app.apiError(w, r, http.StatusUnprocessableEntity, "statement_generation_failed", err.Error(), response.APIError{}, nil)
		return
	}
	if err := response.JSON(w, http.StatusOK, generateStatementResponse{SQL: generated}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) applyConnectionDDL(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	conn := contextGetConnection(r)
	ws := contextGetWorkspace(r)
	if !app.hasAnyConnectionRuntimePermission(r, org.ID, ws.OwnerType, conn.ID,
		access.PermConnExecute, access.PermConnDDL) {
		app.notPermitted(w, r)
		return
	}

	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return
	}
	if session.caps.DDL == nil {
		app.logInfo(r, "structured DDL unsupported", slog.Int64("connection_id", conn.ID))
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support structured DDL.", nil)
		return
	}

	var input ddl.Request
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	if err := ddl.Validate(input, *session.caps.DDL); err != nil {
		app.apiError(w, r, http.StatusUnprocessableEntity, "invalid_schema_edit", err.Error(), response.APIError{}, nil)
		return
	}
	tx, err := app.runtime.ApplyDDL(r.Context(), runtimeScope(r), session.id, input)
	if err != nil {
		var target *execution.TargetError
		switch {
		case errors.Is(err, execution.ErrDDLUnsupported):
			app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support structured DDL.", nil)
		case errors.Is(err, execution.ErrInvalidDDL):
			app.apiError(w, r, http.StatusUnprocessableEntity, "invalid_schema_edit", err.Error(), response.APIError{}, nil)
		case errors.As(err, &target) && !isRequestCanceled(r, err):
			app.apiError(w, r, http.StatusUnprocessableEntity, "schema_edit_failed", target.Error(), response.APIError{}, nil)
		default:
			app.executionError(w, r, err)
		}
		return
	}

	app.logInfo(r, "DDL applied",
		slog.String("session_id", string(session.id)),
		slog.Int64("connection_id", conn.ID),
		slog.String("operation", string(input.Operation)),
	)

	navConn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	mode := "ephemeral"
	if navConn.Persistent {
		mode = "persistent"
	}
	out := schemaEditResponse{
		Applied:     true,
		Schema:      schemaStatusResponse{Status: "available", Mode: mode},
		Transaction: newTransactionStatusView(tx),
	}
	if tree, ok := app.optionalNavigatorTree(conn); ok {
		root := input.Scope
		if input.Ref != nil {
			root = input.Ref.Path()
		}
		listings, refreshErr := app.schemaNavigator.Refresh(r.Context(), navConn, tree, app.navigatorLive(r, session), root)
		if refreshErr != nil {
			app.logWarn(r, "schema edit listing refresh failed",
				slog.Int64("connection_id", conn.ID),
				slog.String("operation", string(input.Operation)),
				slog.Any("error", refreshErr),
			)
			if invalidateErr := app.schemaNavigator.Invalidate(r.Context(), navConn, root); invalidateErr != nil {
				app.logWarn(r, "schema edit cache invalidation failed", slog.Int64("connection_id", conn.ID), slog.Any("error", invalidateErr))
			}
			out.Schema = schemaStatusResponse{Status: "refresh_failed", Mode: mode, Stale: true}
		} else {
			for _, listing := range listings {
				out.Listings = append(out.Listings, listingView(listing))
			}
		}
	}
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjects(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	var input objectsRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	if !app.sessionInspectsSchema(w, r, session) {
		return
	}
	conn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	objects, err := app.schemaNavigator.Objects(r.Context(), conn, app.sessionLive(r, session), input.Refs)
	if err != nil {
		app.navigatorError(w, r, err)
		return
	}
	app.logDebug(r, "schema objects returned",
		slog.Int64("connection_id", conn.ID),
		slog.Int("requested_ref_count", len(input.Refs)),
		slog.Int("object_count", len(objects)),
		slog.Bool("session", session != nil),
	)
	if objects == nil {
		objects = []metadata.Object{}
	}
	if err := response.JSON(w, http.StatusOK, objectsResponse{Objects: objects}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjectDefinition(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	ref, err := schemaObjectRefQuery(r)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	if session == nil {
		app.sessionRequired(w, r)
		return
	}
	if !session.caps.Definitions {
		app.logInfo(r, "schema object definitions unsupported", slog.Int64("connection_id", contextGetConnection(r).ID))
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support on-demand object definitions.", nil)
		return
	}
	descriptor, err := app.runtime.InspectDefinition(r.Context(), runtimeScope(r), session.id, ref)
	if err != nil {
		app.navigatorError(w, r, err)
		return
	}
	app.logDebug(r, "schema object definition returned",
		slog.String("session_id", string(session.id)),
		slog.String("kind", ref.Kind),
		slog.Bool("found", descriptor != nil),
	)
	if err := response.JSON(w, http.StatusOK, objectDefinitionResponse{Descriptor: descriptor}); err != nil {
		app.serverError(w, r, err)
	}
}

func schemaObjectRefQuery(r *http.Request) (metadata.ObjectRef, error) {
	scope, err := schemaScopeQuery(r)
	if err != nil {
		return metadata.ObjectRef{}, err
	}
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	if kind == "" || name == "" {
		return metadata.ObjectRef{}, errors.New("kind and name query parameters are required")
	}
	return metadata.ObjectRef{Scope: scope, Kind: kind, Name: name}, nil
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
