package web

import (
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

func engineToView(eng dbengine.Engine) engineView {
	set := eng.Capabilities()
	v := engineView{
		ID:           string(eng.ID()),
		DisplayName:  eng.DisplayName(),
		Dialect:      string(eng.Dialect()),
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
	for _, eng := range engines {
		views = append(views, engineToView(eng))
	}
	if err := response.JSON(w, http.StatusOK, enginesResponse{Engines: views}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEngine(w http.ResponseWriter, r *http.Request) {
	eng, err := dbengine.New(chi.URLParam(r, "engine_id"))
	if err != nil {
		app.errorMessage(w, r, http.StatusNotFound, "Unknown engine.", nil)
		return
	}
	if err := response.JSON(w, http.StatusOK, engineToView(eng)); err != nil {
		app.serverError(w, r, err)
	}
}
