package web

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/dbengine/schema"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type schemaSpecResponse struct {
	Spec schema.SchemaSpec `json:"spec"`
}

type catalogResponse struct {
	Catalog *schema.Catalog `json:"catalog"`
}

type objectsRequest struct {
	Refs []schema.ObjectRef `json:"refs"`
}

type objectsResponse struct {
	Objects []schema.Object `json:"objects"`
}

type refreshRequest struct {
	Ref *schema.ObjectRef `json:"ref"`
}

type schemaStatusResponse struct {
	Status string `json:"status"`
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
func (app *application) resolveSchemaInspector(w http.ResponseWriter, r *http.Request) (*connection.Session, schema.SchemaInspector, bool) {
	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return nil, nil, false
	}
	inspector, ok := session.Conn.(schema.SchemaInspector)
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

func (app *application) getConnectionSchemaSpec(w http.ResponseWriter, r *http.Request) {
	session, inspector, ok := app.resolveSchemaInspector(w, r)
	if !ok {
		return
	}
	spec := app.schemaService.Spec(inspector)
	app.logDebug(r, "schema spec returned",
		slog.String("session_id", session.ID),
		slog.String("dialect", spec.Dialect),
		slog.Int("kind_count", len(spec.Kinds)),
	)
	if err := response.JSON(w, http.StatusOK, schemaSpecResponse{Spec: spec}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaCatalog(w http.ResponseWriter, r *http.Request) {
	session, inspector, ok := app.resolveSchemaInspector(w, r)
	if !ok {
		return
	}
	cat, err := app.schemaService.Catalog(r.Context(), session.ConnectionID, inspector)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logDebug(r, "schema catalog returned",
		slog.String("session_id", session.ID),
		slog.String("dialect", cat.Dialect),
		slog.Int("namespace_count", len(cat.Namespaces)),
	)
	if err := response.JSON(w, http.StatusOK, catalogResponse{Catalog: cat}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjects(w http.ResponseWriter, r *http.Request) {
	session, inspector, ok := app.resolveSchemaInspector(w, r)
	if !ok {
		return
	}
	var input objectsRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
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
		app.logInfo(r, "schema object cache refresh requested",
			slog.String("session_id", session.ID),
			slog.String("connection_id", session.ConnectionID),
			slog.String("kind", input.Ref.Kind),
			slog.String("namespace", input.Ref.Namespace),
			slog.String("name", input.Ref.Name),
		)
	} else {
		app.schemaService.RefreshConnection(session.ConnectionID)
		app.logInfo(r, "schema connection cache refresh requested",
			slog.String("session_id", session.ID),
			slog.String("connection_id", session.ConnectionID),
		)
	}
	if err := response.JSON(w, http.StatusOK, schemaStatusResponse{Status: "ok"}); err != nil {
		app.serverError(w, r, err)
	}
}
