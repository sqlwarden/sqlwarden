package web

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/response"
)

type engineView struct {
	ID           string                       `json:"id"`
	DisplayName  string                       `json:"display_name"`
	Dialect      string                       `json:"dialect"`
	Capabilities map[dbengine.Capability]bool `json:"capabilities"`
	Schema       *schemaSpecPayload           `json:"schema,omitempty"`
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

func engineToView(set dbengine.CapabilitySet) engineView {
	v := engineView{
		ID:           string(set.Engine.ID),
		DisplayName:  set.Engine.DisplayName,
		Dialect:      string(set.Engine.Dialect),
		Capabilities: set.Capabilities,
	}
	if set.Schema != nil {
		v.Schema = &schemaSpecPayload{Dialect: set.Schema.Dialect, Kinds: set.Schema.Kinds}
	}
	return v
}

func (app *application) listEngines(w http.ResponseWriter, r *http.Request) {
	engines := dbengine.Engines()
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
	set, ok := dbengine.Describe(engineID)
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
