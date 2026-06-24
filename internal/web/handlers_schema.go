package web

import (
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/schema"
)

type capabilitiesResponse struct {
	Capabilities schema.DriverCapabilities `json:"capabilities"`
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
		app.notPermitted(w, r)
		return nil, false
	}

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return nil, false
	}
	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return nil, false
	}
	if session.AccountID != strconv.FormatInt(account.ID, 10) ||
		session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return nil, false
	}
	return session, true
}

// resolveIntrospector resolves the active database session and checks whether
// the concrete driver supports schema introspection.
func (app *application) resolveIntrospector(w http.ResponseWriter, r *http.Request) (*connection.Session, schema.Introspector, bool) {
	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return nil, nil, false
	}
	intr, ok := session.Driver.(schema.Introspector)
	if !ok {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema introspection.", nil)
		return nil, nil, false
	}
	return session, intr, true
}

func (app *application) getConnectionSchemaCapabilities(w http.ResponseWriter, r *http.Request) {
	_, intr, ok := app.resolveIntrospector(w, r)
	if !ok {
		return
	}
	caps := app.schemaService.Capabilities(intr)
	if err := response.JSON(w, http.StatusOK, capabilitiesResponse{Capabilities: caps}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaCatalog(w http.ResponseWriter, r *http.Request) {
	session, intr, ok := app.resolveIntrospector(w, r)
	if !ok {
		return
	}
	cat, err := app.schemaService.Catalog(r.Context(), session.ConnectionID, intr)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, catalogResponse{Catalog: cat}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjects(w http.ResponseWriter, r *http.Request) {
	session, intr, ok := app.resolveIntrospector(w, r)
	if !ok {
		return
	}
	var input objectsRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	objects, err := app.schemaService.Objects(r.Context(), session.ConnectionID, input.Refs, intr)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, objectsResponse{Objects: objects}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshConnectionSchema(w http.ResponseWriter, r *http.Request) {
	session, _, ok := app.resolveIntrospector(w, r)
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
	} else {
		app.schemaService.RefreshConnection(session.ConnectionID)
	}
	if err := response.JSON(w, http.StatusOK, schemaStatusResponse{Status: "ok"}); err != nil {
		app.serverError(w, r, err)
	}
}
