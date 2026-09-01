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

	persistent, err := app.persistentSchemaMode(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	var (
		directory *metadata.Directory
		objects   []metadata.Object
		version   string
		mode      string
	)

	if persistent {
		if !app.authorizeSchemaAccess(w, r) {
			return
		}
		conn := contextGetConnection(r)
		snapshot, dir, found, lookupErr := app.schemaSnapshots.Active(r.Context(), conn.ID)
		if lookupErr != nil {
			app.serverError(w, r, lookupErr)
			return
		}
		if !found {
			app.writeSnapshotPending(w, r)
			return
		}
		objs, objErr := app.schemaSnapshots.AllObjects(r.Context(), snapshot.ID)
		if objErr != nil {
			app.serverError(w, r, objErr)
			return
		}
		directory, objects, version, mode = dir, objs, snapshot.ID, "persistent"
	} else {
		session, inspector, ok := app.resolveSchemaInspector(w, r)
		if !ok {
			return
		}
		dir, dirErr := app.schemaService.Directory(r.Context(), session.ConnectionID, inspector)
		if dirErr != nil {
			app.serverError(w, r, dirErr)
			return
		}
		objs, objErr := app.schemaService.Objects(r.Context(), session.ConnectionID, directoryObjectRefs(dir), inspector)
		if objErr != nil {
			app.serverError(w, r, objErr)
			return
		}
		directory, objects, version, mode = dir, objs, dir.GeneratedAt.UTC().Format(time.RFC3339Nano), "ephemeral"
	}

	out := projectCompletionIndex(directory, objects, version)

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

	if directory != nil {
		out.DefaultSchema = directory.DefaultScope.Name("schema")
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
		out.Objects = append(out.Objects, completionIndexObject{
			Schema: schema, Name: obj.Ref.Name, Kind: obj.Ref.Kind,
		})
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
