package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	schemaapp "github.com/sqlwarden/internal/schema"
)

type navigatorItemView struct {
	Kind       string             `json:"kind"`
	Name       string             `json:"name"`
	Path       metadata.ScopePath `json:"path"`
	Attributes map[string]any     `json:"attributes,omitempty"`
	System     bool               `json:"system"`
	Current    bool               `json:"current"`
}

type navigatorListingView struct {
	Path      metadata.ScopePath  `json:"path"`
	Folder    string              `json:"folder"`
	Items     []navigatorItemView `json:"items"`
	FetchedAt time.Time           `json:"fetched_at"`
	Source    string              `json:"source"`
}

type schemaTreeResponse struct {
	metadata.Tree
	Editor     *ddl.Spec       `json:"editor,omitempty"`
	Statements *statement.Spec `json:"statements,omitempty"`
}

type navigatorRefreshRequest struct {
	Path metadata.ScopePath `json:"path"`
}

func listingView(listing schemaapp.Listing) navigatorListingView {
	items := make([]navigatorItemView, 0, len(listing.Items))
	for _, item := range listing.Items {
		items = append(items, navigatorItemView{
			Kind: item.Kind, Name: item.Name, Attributes: item.Attributes, System: item.System, Current: item.Current,
			Path: listing.Parent.Child(metadata.ScopeSegment{Kind: item.Kind, Name: item.Name}),
		})
	}
	return navigatorListingView{Path: listing.Parent, Folder: listing.Folder, Items: items, FetchedAt: listing.FetchedAt, Source: listing.Source}
}

func (app *application) navigatorTree(w http.ResponseWriter, r *http.Request) (metadata.Tree, bool) {
	conn := contextGetConnection(r)
	tree, ok := app.optionalNavigatorTree(conn)
	if !ok {
		app.logWarn(r, "schema navigator unsupported", slog.Int64("connection_id", conn.ID))
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support the schema navigator.", nil)
	}
	return tree, ok
}

func (app *application) optionalNavigatorTree(conn database.Connection) (metadata.Tree, bool) {
	set, ok := engine.Describe(conn.Driver)
	if !ok || set.Tree == nil {
		return metadata.Tree{}, false
	}
	return *set.Tree, true
}

func (app *application) navigatorConnection(r *http.Request) (schemaapp.Connection, error) {
	conn := contextGetConnection(r)
	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		return schemaapp.Connection{}, err
	}
	return schemaapp.Connection{
		ID:               conn.ID,
		DefaultScope:     conn.DefaultScope,
		ShowSystem:       conn.ShowSystemSchemas,
		ShowAllDatabases: conn.ShowAllDatabases,
		Persistent:       persistent,
	}, nil
}

// optionalSchemaSession resolves X-Warden-Session when present. A missing or
// expired session is not an error: cached reads still succeed and uncached
// reads return session_required.
func (app *application) optionalSchemaSession(w http.ResponseWriter, r *http.Request) (*connection.Session, bool) {
	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		return nil, true
	}
	session, ok := app.connManager.Get(sessionID)
	if !ok {
		return nil, true
	}
	account := contextGetAccount(r)
	conn := contextGetConnection(r)
	if session.AccountID != strconv.FormatInt(account.ID, 10) || session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.logWarn(r, "schema session scope mismatch", slog.Int64("connection_id", conn.ID))
		app.notPermitted(w, r)
		return nil, false
	}
	return session, true
}

func navigatorLive(session *connection.Session) metadata.SchemaInspector {
	if session == nil {
		return nil
	}
	inspector, ok := session.Conn.(metadata.SchemaInspector)
	if !ok {
		return nil
	}
	return inspector
}

// sessionRequired answers 410 when the request named a session that no longer
// exists, so clients drop the dead session instead of reporting it connected.
func (app *application) sessionRequired(w http.ResponseWriter, r *http.Request) {
	if sessionID := r.Header.Get("X-Warden-Session"); sessionID != "" {
		if _, ok := app.connManager.Get(sessionID); !ok {
			app.logInfo(r, "schema read with expired session", slog.Int64("connection_id", contextGetConnection(r).ID))
			app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
			return
		}
	}
	app.logInfo(r, "schema read requires session", slog.Int64("connection_id", contextGetConnection(r).ID))
	app.apiError(w, r, http.StatusConflict, "session_required", "Connect to this database to load schema objects.", response.APIError{}, nil)
}

func (app *application) navigatorError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, schemaapp.ErrSessionRequired):
		app.sessionRequired(w, r)
	case errors.Is(err, schemaapp.ErrUnknownFolder):
		app.errorMessage(w, r, http.StatusBadRequest, "Unknown folder for this node.", nil)
	default:
		app.logWarn(r, "schema navigator load failed", slog.Int64("connection_id", contextGetConnection(r).ID), slog.Any("error", err))
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "Schema metadata could not be loaded from the database.", nil)
	}
}

func (app *application) getConnectionSchemaTree(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	tree, ok := app.navigatorTree(w, r)
	if !ok {
		return
	}
	out := schemaTreeResponse{Tree: tree}
	if driver, err := engine.New(contextGetConnection(r).Driver); err == nil {
		if executor, ok := driver.(ddl.Executor); ok {
			spec := executor.DDLSpec()
			out.Editor = &spec
		}
		if generator, ok := driver.(statement.Generator); ok {
			spec := generator.StatementSpec()
			out.Statements = &spec
		}
	}
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getConnectionSchemaNodes(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	tree, ok := app.navigatorTree(w, r)
	if !ok {
		return
	}
	folder := r.URL.Query().Get("folder")
	if folder == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "folder query parameter is required.", nil)
		return
	}
	var parent metadata.ScopePath
	if raw := r.URL.Query().Get("path"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &parent); err != nil {
			app.errorMessage(w, r, http.StatusBadRequest, "path query parameter must be a JSON array of segments.", nil)
			return
		}
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	conn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	start := time.Now()
	listing, err := app.schemaNavigator.Children(r.Context(), conn, tree, navigatorLive(session), parent, folder)
	if err != nil {
		app.navigatorError(w, r, err)
		return
	}
	app.logDebug(r, "schema listing served",
		slog.Int64("connection_id", conn.ID),
		slog.String("folder", folder),
		slog.Int("depth", parent.Depth()),
		slog.String("source", listing.Source),
		slog.Int("items", len(listing.Items)),
		slog.Duration("duration", time.Since(start)),
	)
	if err := response.JSON(w, http.StatusOK, listingView(listing)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshConnectionSchemaNodes(w http.ResponseWriter, r *http.Request) {
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	tree, ok := app.navigatorTree(w, r)
	if !ok {
		return
	}
	var input navigatorRefreshRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	live := navigatorLive(session)
	if live == nil {
		app.sessionRequired(w, r)
		return
	}
	conn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	start := time.Now()
	listings, err := app.schemaNavigator.Refresh(r.Context(), conn, tree, live, input.Path)
	if err != nil {
		app.navigatorError(w, r, err)
		return
	}
	app.completionService.InvalidateConnection(strconv.FormatInt(conn.ID, 10))
	app.logInfo(r, "schema subtree refreshed",
		slog.Int64("connection_id", conn.ID),
		slog.Int("depth", input.Path.Depth()),
		slog.Int("listings", len(listings)),
		slog.Duration("duration", time.Since(start)),
	)
	views := make([]navigatorListingView, 0, len(listings))
	for _, listing := range listings {
		views = append(views, listingView(listing))
	}
	if err := response.JSON(w, http.StatusOK, map[string]any{"items": views}); err != nil {
		app.serverError(w, r, err)
	}
}
