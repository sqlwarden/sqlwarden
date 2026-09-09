package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/response"
)

// lazySchemaScope reports whether the directory lists a scope whose objects
// have not been inspected. Persistent handlers use this to choose live
// inspection for metadata intentionally absent from the saved snapshot.
func lazySchemaScope(directory *metadata.Directory, scope metadata.ScopePath) bool {
	for _, node := range directory.ScopeNodes() {
		if node.Path == scope {
			return node.Lazy
		}
	}
	return false
}

// withLiveSchemaInspector runs an inspection with the caller's existing session
// in ephemeral mode, or an authorized, short-lived target connection in persistent
// mode. It owns the temporary connection and timeout, and writes resolution and
// callback errors to the response. The callback must return response-write errors
// and must not retain the inspector after returning.
func (app *application) withLiveSchemaInspector(w http.ResponseWriter, r *http.Request, persistent bool, run func(context.Context, string, metadata.SchemaInspector) error) {
	if !persistent {
		session, inspector, ok := app.resolveSchemaInspector(w, r)
		if !ok {
			return
		}
		if err := run(r.Context(), session.ConnectionID, inspector); err != nil {
			app.serverError(w, r, err)
		}
		return
	}
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), lazyDefinitionTimeout)
	defer cancel()
	driver, err := app.openTargetDriver(ctx, contextGetConnection(r), contextGetWorkspace(r))
	if err != nil {
		app.schemaSyncHTTPError(w, r, err)
		return
	}
	defer driver.Close()
	inspector, ok := driver.(metadata.SchemaInspector)
	if !ok {
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema inspection.", nil)
		return
	}
	if err := run(ctx, strconv.FormatInt(contextGetConnection(r).ID, 10), inspector); err != nil {
		app.serverError(w, r, err)
	}
}

// getConnectionSchemaScopeDirectory serves the directory endpoint's scoped
// request. It checks schema access, reuses cached listings in persistent mode,
// and requires a valid session in ephemeral mode even when metadata is cached.
func (app *application) getConnectionSchemaScopeDirectory(w http.ResponseWriter, r *http.Request, scope metadata.ScopePath, persistent bool) {
	if scope == "" {
		app.badRequest(w, r, errors.New("scope is required"))
		return
	}
	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	// Ephemeral mode must validate the caller's session even on a cache hit.
	showSystem := contextGetConnection(r).ShowSystemSchemas
	if persistent {
		if cached, ok := app.schemaService.CachedScopeDirectory(strconv.FormatInt(contextGetConnection(r).ID, 10), scope); ok {
			cached = cached.WithSystemScopes(showSystem)
			if err := response.JSON(w, http.StatusOK, directoryResponse{Directory: cached}); err != nil {
				app.serverError(w, r, err)
			}
			return
		}
	}
	app.withLiveSchemaInspector(w, r, persistent, func(ctx context.Context, connID string, inspector metadata.SchemaInspector) error {
		if !inspector.SchemaSpec().BrowseScopes {
			app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support browsing additional scopes.", nil)
			return nil
		}
		directory, err := app.schemaService.DirectoryInScope(ctx, connID, scope, inspector)
		if err != nil {
			return err
		}
		app.logDebug(r, "schema scope directory returned", slog.Int("scope_count", len(directory.Roots)))
		directory = directory.WithSystemScopes(showSystem)
		return response.JSON(w, http.StatusOK, directoryResponse{Directory: directory})
	})
}

// liveSchemaObjects resolves persistent-mode object details from the cache,
// inspecting cache misses through a temporary target connection. Callers must
// authorize schema access before calling because a full cache hit opens no
// connection. A false result means an error response has already been written.
func (app *application) liveSchemaObjects(w http.ResponseWriter, r *http.Request, refs []metadata.ObjectRef) ([]metadata.Object, bool) {
	connID := strconv.FormatInt(contextGetConnection(r).ID, 10)
	cached := app.schemaService.CachedObjects(connID, refs)
	if len(cached) == len(refs) {
		return cached, true
	}
	var objects []metadata.Object
	success := false
	app.withLiveSchemaInspector(w, r, true, func(ctx context.Context, connID string, inspector metadata.SchemaInspector) error {
		var err error
		objects, err = app.schemaService.Objects(ctx, connID, refs, inspector)
		success = err == nil
		return err
	})
	return objects, success
}

// getLiveSchemaRelationships serves relationships for a persistent-mode scope
// omitted from the saved snapshot, using the driver's relationship capability
// and the shared schema cache through an authorized temporary connection.
func (app *application) getLiveSchemaRelationships(w http.ResponseWriter, r *http.Request, scope metadata.ScopePath) {
	app.withLiveSchemaInspector(w, r, true, func(ctx context.Context, connID string, inspector metadata.SchemaInspector) error {
		relationships, ok := inspector.(metadata.RelationshipInspector)
		if !ok {
			app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema relationships.", nil)
			return nil
		}
		graph, err := app.schemaService.Relationships(ctx, connID, scope, relationships)
		if err != nil {
			return err
		}
		return response.JSON(w, http.StatusOK, relationshipsResponse{Graph: graph})
	})
}
