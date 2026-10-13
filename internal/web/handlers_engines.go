package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/catalog"
	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/response"
)

// engineError maps engine catalog failures.
func (app *application) engineError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrUnknownEngine), errors.Is(err, queryapp.ErrUnknownEngine):
		app.errorMessage(w, r, http.StatusNotFound, "Unknown engine.", nil)
	case errors.Is(err, queryapp.ErrVocabularyUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This engine does not provide a completion vocabulary.", nil)
	case errors.Is(err, access.ErrForbidden):
		app.notPermitted(w, r)
	default:
		app.serverError(w, r, err)
	}
}

func (app *application) listEngines(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	out, err := app.catalogService().Engines(r.Context(), p)
	if err != nil {
		app.engineError(w, r, err)
		return
	}
	app.logDebug(r, "database engines listed", slog.Int("engine_count", len(out.Engines)))
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEngine(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	engineID := chi.URLParam(r, "engine_id")
	view, err := app.catalogService().Engine(r.Context(), p, engineID)
	if err != nil {
		if errors.Is(err, catalog.ErrUnknownEngine) {
			app.logWarn(r, "database engine lookup failed", slog.String("engine_id", engineID))
		}
		app.engineError(w, r, err)
		return
	}
	app.logDebug(r, "database engine described",
		slog.String("engine_id", view.ID),
		slog.String("dialect", view.Dialect),
		slog.Int("capability_count", len(view.Capabilities)),
	)
	if err := response.JSON(w, http.StatusOK, view); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEngineCompletionVocabulary(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	vocabulary, err := app.queryService().CompletionVocabulary(r.Context(), p, chi.URLParam(r, "engine_id"))
	if err != nil {
		app.engineError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, vocabulary); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getEngineConnectionFields(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	fields, err := app.catalogService().EngineConnectionFields(r.Context(), p, chi.URLParam(r, "driver"))
	if err != nil {
		app.engineError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, fields); err != nil {
		app.serverError(w, r, err)
	}
}
