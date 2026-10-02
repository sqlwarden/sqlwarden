package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/sqlwarden/internal/engine/completer"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/response"
)

type completionIndexObject struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Score  int    `json:"score"`
}

type completionIndexColumn struct {
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Nullable bool   `json:"nullable"`
}

type completionIndexResponse struct {
	Version       string `json:"version"`
	DefaultSchema string `json:"default_schema"`
	// SearchSchemas orders the schemas unqualified relation names resolve
	// in; a database-level scope contributes "".
	SearchSchemas []string `json:"search_schemas"`
	// DefaultScopeRelationsListed is true when every relation folder of every
	// search scope has a cached listing.
	DefaultScopeRelationsListed bool                    `json:"default_scope_relations_listed"`
	ColumnScore                 int                     `json:"column_score"`
	Schemas                     []string                `json:"schemas"`
	Objects                     []completionIndexObject `json:"objects"`
	Columns                     []completionIndexColumn `json:"columns"`
}

func (app *application) getConnectionCompletionIndex(w http.ResponseWriter, r *http.Request) {
	started := time.Now()

	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	session, ok := app.optionalSchemaSession(w, r)
	if !ok {
		return
	}
	conn := contextGetConnection(r)
	navConn, err := app.navigatorConnection(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	mode := "ephemeral"
	if navConn.Persistent {
		mode = "persistent"
	}
	var view *metadata.CompletionView
	if tree, ok := app.optionalNavigatorTree(conn); ok {
		view, err = app.schemaNavigator.CompletionView(r.Context(), navConn, tree, navigatorLive(session))
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	} else {
		view = metadata.NewCompletionView(metadata.Tree{}, conn.DefaultScope, "", nil, nil)
	}
	out, err := projectCompletionIndex(view)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logDebug(r, "completion index returned",
		slog.String("mode", mode),
		slog.Int("schema_count", len(out.Schemas)),
		slog.Int("object_count", len(out.Objects)),
		slog.Int("column_count", len(out.Columns)),
		slog.Int64("duration_ms", time.Since(started).Milliseconds()),
	)

	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func relationKindsOf(tree metadata.Tree, scope metadata.ScopePath) []string {
	node, ok := tree.Node(tree.NodeKindOf(scope))
	if !ok || !node.Scope {
		return nil
	}
	var kinds []string
	for _, folder := range node.Folders {
		for _, kind := range append([]string{folder.Child}, folder.MixedKinds...) {
			if child, ok := tree.Node(kind); ok && child.Relational && !slices.Contains(kinds, kind) {
				kinds = append(kinds, kind)
			}
		}
	}
	return kinds
}

// searchRelationScopes keeps the search scopes that hold relations. A
// fallback scope is dropped once its parent's listing is loaded without it.
func searchRelationScopes(view *metadata.CompletionView) []metadata.ScopePath {
	var out []metadata.ScopePath
	for i, scope := range view.SearchScopes() {
		if i > 0 {
			if siblings, loaded := view.Scopes(scope.Parent()); loaded && !slices.Contains(siblings, scope) {
				continue
			}
		}
		if len(relationKindsOf(view.Tree(), scope)) > 0 {
			out = append(out, scope)
		}
	}
	return out
}

func searchRelationsListed(view *metadata.CompletionView, scopes []metadata.ScopePath) bool {
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if len(view.ObjectDemands(scope, relationKindsOf(view.Tree(), scope)...)) > 0 {
			return false
		}
	}
	return true
}

func projectCompletionIndex(view *metadata.CompletionView) (completionIndexResponse, error) {
	scopes := searchRelationScopes(view)
	searchSchemas := []string{}
	for _, scope := range scopes {
		if name := scope.Name("schema"); !slices.Contains(searchSchemas, name) {
			searchSchemas = append(searchSchemas, name)
		}
	}
	out := completionIndexResponse{
		DefaultSchema:               view.DefaultScope().Name("schema"),
		SearchSchemas:               searchSchemas,
		DefaultScopeRelationsListed: searchRelationsListed(view, scopes),
		ColumnScore:                 completer.KindScore("column"),
		Schemas:                     []string{},
		Objects:                     []completionIndexObject{},
		Columns:                     []completionIndexColumn{},
	}
	schemas := map[string]struct{}{}
	for _, path := range view.ScopePaths() {
		if schema := path.Name("schema"); schema != "" {
			schemas[schema] = struct{}{}
		}
	}
	for _, ref := range view.Refs() {
		schema := ref.Scope.Name("schema")
		out.Objects = append(out.Objects, completionIndexObject{Schema: schema, Name: ref.Name, Kind: ref.Kind, Score: completer.KindScore(ref.Kind)})
		columns, _ := view.Columns(ref)
		for _, column := range columns {
			out.Columns = append(out.Columns, completionIndexColumn{
				Schema: schema, Table: ref.Name, Name: column.Name, Type: column.DataType, Nullable: column.Nullable,
			})
		}
	}
	for schema := range schemas {
		out.Schemas = append(out.Schemas, schema)
	}
	sort.Strings(out.Schemas)
	raw, err := json.Marshal(out)
	if err != nil {
		return completionIndexResponse{}, fmt.Errorf("hash completion index: %w", err)
	}
	sum := sha256.Sum256(raw)
	out.Version = hex.EncodeToString(sum[:])
	return out, nil
}
