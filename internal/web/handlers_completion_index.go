package web

import (
	"log/slog"
	"net/http"
	"sort"
	"time"

	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/response"
)

type completionIndexObject struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

type completionIndexColumn struct {
	Schema   string `json:"schema"`
	Table    string `json:"table"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Nullable bool   `json:"nullable"`
}

type completionIndexResponse struct {
	Version       string                  `json:"version"`
	DefaultSchema string                  `json:"default_schema"`
	Schemas       []string                `json:"schemas"`
	Objects       []completionIndexObject `json:"objects"`
	Columns       []completionIndexColumn `json:"columns"`
}

func (app *application) getConnectionCompletionIndex(w http.ResponseWriter, r *http.Request) {
	started := time.Now()

	if !app.authorizeSchemaAccess(w, r) {
		return
	}
	if _, ok := app.optionalSchemaSession(w, r); !ok {
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
	var set *metadata.MetadataSet
	if tree, ok := app.optionalNavigatorTree(conn); ok {
		set, err = app.schemaNavigator.CompletionMetadata(r.Context(), navConn, tree)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	} else {
		set = &metadata.MetadataSet{Directory: &metadata.Directory{DefaultScope: conn.DefaultScope}}
	}
	out := projectCompletionIndex(set.Directory, set.Objects, set.Version)

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

func projectCompletionIndex(directory *metadata.Directory, objects []metadata.Object, version string) completionIndexResponse {
	out := completionIndexResponse{
		Version: version,
		Objects: []completionIndexObject{},
		Columns: []completionIndexColumn{},
		Schemas: []string{},
	}
	schemaSet := map[string]struct{}{}
	seen := map[metadata.ObjectRef]bool{}
	addObject := func(ref metadata.ObjectRef) {
		if seen[ref] {
			return
		}
		seen[ref] = true
		out.Objects = append(out.Objects, completionIndexObject{Schema: ref.Scope.Name("schema"), Name: ref.Name, Kind: ref.Kind})
	}

	if directory != nil {
		out.DefaultSchema = directory.DefaultScope.Name("schema")
		for _, ref := range directory.ObjectRefs() {
			addObject(ref)
		}
		for _, node := range directory.ScopeNodes() {
			if schema := node.Path.Name("schema"); schema != "" {
				schemaSet[schema] = struct{}{}
			}
		}
	}

	for _, obj := range objects {
		schema := obj.Ref.Scope.Name("schema")
		if schema != "" {
			schemaSet[schema] = struct{}{}
		}
		addObject(obj.Ref)
		if obj.Relational != nil {
			for _, col := range obj.Relational.Columns {
				out.Columns = append(out.Columns, completionIndexColumn{
					Schema: schema, Table: obj.Ref.Name, Name: col.Name,
					Type: col.DataType, Nullable: col.Nullable,
				})
			}
		}
	}

	for schema := range schemaSet {
		out.Schemas = append(out.Schemas, schema)
	}
	sort.Strings(out.Schemas)

	return out
}
