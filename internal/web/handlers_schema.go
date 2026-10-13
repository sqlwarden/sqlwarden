package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/sqlwarden/internal/execution"
	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type objectsRequest struct {
	Refs []queryapp.ObjectRef `json:"refs"`
}

type objectsResponse struct {
	Objects []queryapp.SchemaObject `json:"objects"`
}

type objectDefinitionResponse struct {
	Descriptor *queryapp.ObjectDescriptor `json:"descriptor"`
}

type relationshipsResponse struct {
	Graph *queryapp.RelationshipGraph `json:"graph"`
}

type generateStatementRequest struct {
	Operation queryapp.StatementOperation `json:"operation"`
	Ref       queryapp.ObjectRef          `json:"ref"`
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

func schemaMode(persistent bool) string {
	if persistent {
		return "persistent"
	}
	return "ephemeral"
}

// schemaError maps schema read failures. Anything unclassified is internal.
func (app *application) schemaError(w http.ResponseWriter, r *http.Request, err error) {
	if !app.schemaDomainError(w, r, err) {
		app.serverError(w, r, err)
	}
}

// schemaRuntimeError maps failures from schema edits, where anything
// unclassified came from the execution runtime.
func (app *application) schemaRuntimeError(w http.ResponseWriter, r *http.Request, err error) {
	if !app.schemaDomainError(w, r, err) {
		app.executionError(w, r, err)
	}
}

func (app *application) schemaDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	connID := slog.Int64("connection_id", contextGetPathIDs(r).ConnectionID)
	var read *queryapp.SchemaReadError
	var edit *queryapp.SchemaEditError
	var generate *queryapp.StatementError
	var failure *execution.Failure
	switch {
	case errors.Is(err, queryapp.ErrSchemaSessionExpired):
		app.logInfo(r, "schema read with expired session", connID)
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
	case errors.Is(err, queryapp.ErrSchemaSessionRequired):
		app.logInfo(r, "schema read requires session", connID)
		app.apiError(w, r, http.StatusConflict, "session_required", "Connect to this database to load schema objects.", response.APIError{}, nil)
	case errors.Is(err, queryapp.ErrSchemaNavigatorUnsupported):
		app.logWarn(r, "schema navigator unsupported", connID)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support the schema navigator.", nil)
	case errors.Is(err, queryapp.ErrSchemaFolderRequired):
		app.errorMessage(w, r, http.StatusBadRequest, "folder query parameter is required.", nil)
	case errors.Is(err, queryapp.ErrSchemaUnknownFolder):
		app.errorMessage(w, r, http.StatusBadRequest, "Unknown folder for this node.", nil)
	case errors.Is(err, execution.ErrSchemaUnsupported):
		app.logInfo(r, "schema inspection unsupported", connID)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema inspection.", nil)
	case errors.Is(err, execution.ErrRelationshipsUnsupported):
		app.logInfo(r, "schema relationships unsupported", connID)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema relationships.", nil)
	case errors.Is(err, execution.ErrDefinitionUnsupported):
		app.logInfo(r, "schema object definitions unsupported", connID)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support on-demand object definitions.", nil)
	case errors.Is(err, execution.ErrDDLUnsupported):
		app.logInfo(r, "structured DDL unsupported", connID)
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support structured DDL.", nil)
	case errors.Is(err, queryapp.ErrStatementsUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support statement generation.", nil)
	case errors.Is(err, queryapp.ErrStatementUnsupported):
		app.apiError(w, r, http.StatusUnprocessableEntity, "statement_generation_failed", "The requested statement is not supported for this object.", response.APIError{}, nil)
	case errors.As(err, &generate):
		app.apiError(w, r, http.StatusUnprocessableEntity, "statement_generation_failed", generate.Error(), response.APIError{}, nil)
	case errors.Is(err, queryapp.ErrSchemaObjectNotFound):
		app.errorMessage(w, r, http.StatusNotFound, "Schema object was not found.", nil)
	case errors.As(err, &edit):
		app.apiError(w, r, http.StatusUnprocessableEntity, edit.Code, edit.Error(), response.APIError{}, nil)
	case errors.As(err, &read) && errors.As(err, &failure) &&
		(failure.Code == execution.FailureSessionNotFound || failure.Code == execution.FailureSessionLost):
		app.executionError(w, r, err)
	case errors.As(err, &read):
		app.logWarn(r, "schema navigator load failed", connID, slog.Any("error", err))
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "Schema metadata could not be loaded from the database.", nil)
	default:
		return app.queryDomainError(w, r, err)
	}
	return true
}

func (app *application) getConnectionSchemaRelationships(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	scope, err := schemaScopeQuery(r)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}
	graph, err := app.queryService().SchemaRelationships(r.Context(), p, connectionRef(r), requestSessionID(r), scope)
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	app.logDebug(r, "schema relationships returned",
		slog.Int64("connection_id", contextGetPathIDs(r).ConnectionID),
		slog.Int("edge_count", len(graph.Relationships)),
		slog.Bool("session", requestSessionID(r) != ""),
	)
	if err := response.JSON(w, http.StatusOK, relationshipsResponse{Graph: graph}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) generateConnectionStatement(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input generateStatementRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	generated, err := app.queryService().GenerateStatement(r.Context(), p, connectionRef(r), requestSessionID(r), input.Operation, input.Ref)
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, generateStatementResponse{SQL: generated}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) applyConnectionDDL(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input queryapp.DDLRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	result, err := app.queryService().ObjectDDL(r.Context(), p, connectionRef(r), requestSessionID(r), input)
	if err != nil {
		app.schemaRuntimeError(w, r, err)
		return
	}
	mode := schemaMode(result.Persistent)
	out := schemaEditResponse{
		Applied:     true,
		Schema:      schemaStatusResponse{Status: "available", Mode: mode},
		Transaction: newTransactionStatusView(result.Transaction),
	}
	if result.RefreshFailed {
		out.Schema = schemaStatusResponse{Status: "refresh_failed", Mode: mode, Stale: true}
	}
	for _, listing := range result.Listings {
		out.Listings = append(out.Listings, listingView(listing))
	}
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjects(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	var input objectsRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	objects, err := app.queryService().SchemaObjects(r.Context(), p, connectionRef(r), requestSessionID(r), input.Refs)
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	app.logDebug(r, "schema objects returned",
		slog.Int64("connection_id", contextGetPathIDs(r).ConnectionID),
		slog.Int("requested_ref_count", len(input.Refs)),
		slog.Int("object_count", len(objects)),
		slog.Bool("session", requestSessionID(r) != ""),
	)
	if objects == nil {
		objects = []queryapp.SchemaObject{}
	}
	if err := response.JSON(w, http.StatusOK, objectsResponse{Objects: objects}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaObjectDefinition(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ref, err := schemaObjectRefQuery(r)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}
	descriptor, err := app.queryService().ObjectDefinition(r.Context(), p, connectionRef(r), requestSessionID(r), ref)
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	app.logDebug(r, "schema object definition returned",
		slog.String("session_id", string(requestSessionID(r))),
		slog.String("kind", ref.Kind),
		slog.Bool("found", descriptor != nil),
	)
	if err := response.JSON(w, http.StatusOK, objectDefinitionResponse{Descriptor: descriptor}); err != nil {
		app.serverError(w, r, err)
	}
}

func schemaObjectRefQuery(r *http.Request) (queryapp.ObjectRef, error) {
	scope, err := schemaScopeQuery(r)
	if err != nil {
		return queryapp.ObjectRef{}, err
	}
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	if kind == "" || name == "" {
		return queryapp.ObjectRef{}, errors.New("kind and name query parameters are required")
	}
	return queryapp.ObjectRef{Scope: scope, Kind: kind, Name: name}, nil
}

func schemaScopeQuery(r *http.Request) (queryapp.ScopePath, error) {
	raw := r.URL.Query().Get("scope")
	if raw == "" {
		return "", errors.New("scope query parameter is required")
	}
	var scope queryapp.ScopePath
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		return "", err
	}
	if scope == "" {
		return "", errors.New("scope query parameter must not be empty")
	}
	return scope, nil
}
