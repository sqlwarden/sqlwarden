package web

import (
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/schema"
)

type connectionSchemaResponse struct {
	Schema *schema.Schema `json:"schema"`
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

func (app *application) getConnectionSchema(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return
	}
	intr, ok := session.Driver.(schema.Introspector)
	if !ok {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema introspection.", nil)
		return
	}
	snap, err := app.schemaService.Get(r.Context(), session.ConnectionID, intr)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, connectionSchemaResponse{Schema: snap}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshConnectionSchema(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveSchemaSession(w, r)
	if !ok {
		return
	}
	intr, ok := session.Driver.(schema.Introspector)
	if !ok {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema introspection.", nil)
		return
	}
	snap, err := app.schemaService.Refresh(r.Context(), session.ConnectionID, intr)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, connectionSchemaResponse{Schema: snap}); err != nil {
		app.serverError(w, r, err)
	}
}
