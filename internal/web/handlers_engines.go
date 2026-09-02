package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	completionapp "github.com/sqlwarden/internal/completion"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/explain"
	"github.com/sqlwarden/internal/engine/statement"
	"github.com/sqlwarden/internal/response"
)

type engineView struct {
	ID           string                     `json:"id"`
	DisplayName  string                     `json:"display_name"`
	Dialect      string                     `json:"dialect"`
	Capabilities map[engine.Capability]bool `json:"capabilities"`
	Schema       *schemaSpecPayload         `json:"schema,omitempty"`
	DDL          *ddl.Spec                  `json:"schema_edit,omitempty"`
	Statements   *statement.Spec            `json:"statements,omitempty"`
	Explain      *explain.Spec              `json:"explain,omitempty"`
	TLS          *engine.TLSSpec            `json:"connection_tls,omitempty"`
}

// schemaSpecPayload mirrors schema.SchemaSpec but lives here so the engines API
// owns its own serialization shape and does not leak engine internals.
type schemaSpecPayload struct {
	Dialect string `json:"dialect"`
	Kinds   any    `json:"kinds"`
}

type enginesResponse struct {
	Engines []engineView `json:"engines"`
}

func engineToView(set engine.CapabilitySet) engineView {
	v := engineView{
		ID:           string(set.Engine.ID),
		DisplayName:  set.Engine.DisplayName,
		Dialect:      string(set.Engine.Dialect),
		Capabilities: set.Capabilities,
		DDL:          set.DDL,
		Statements:   set.Statements,
		Explain:      set.Explain,
		TLS:          set.TLS,
	}
	if set.Schema != nil {
		v.Schema = &schemaSpecPayload{Dialect: set.Schema.Dialect, Kinds: set.Schema.Kinds}
	}
	return v
}

func (app *application) listEngines(w http.ResponseWriter, r *http.Request) {
	engines := engine.Engines()
	views := make([]engineView, 0, len(engines))
	for _, set := range engines {
		views = append(views, engineToView(set))
	}
	app.logDebug(r, "database engines listed", slog.Int("engine_count", len(views)))
	if err := response.JSON(w, http.StatusOK, enginesResponse{Engines: views}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEngine(w http.ResponseWriter, r *http.Request) {
	engineID := chi.URLParam(r, "engine_id")
	set, ok := engine.Describe(engineID)
	if !ok {
		app.logWarn(r, "database engine lookup failed", slog.String("engine_id", engineID))
		app.errorMessage(w, r, http.StatusNotFound, "Unknown engine.", nil)
		return
	}
	app.logDebug(r, "database engine described",
		slog.String("engine_id", string(set.Engine.ID)),
		slog.String("dialect", string(set.Engine.Dialect)),
		slog.Int("capability_count", len(set.Capabilities)),
	)
	if err := response.JSON(w, http.StatusOK, engineToView(set)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEngineCompletionVocabulary(w http.ResponseWriter, r *http.Request) {
	engineID := chi.URLParam(r, "engine_id")
	if _, ok := engine.Describe(engineID); !ok {
		app.errorMessage(w, r, http.StatusNotFound, "Unknown engine.", nil)
		return
	}
	vocabulary, err := app.completionService.Vocabulary(engineID)
	if errors.Is(err, completionapp.ErrUnsupported) {
		app.errorMessage(w, r, http.StatusNotImplemented, "This engine does not provide a completion vocabulary.", nil)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, vocabulary); err != nil {
		app.serverError(w, r, err)
	}
}
