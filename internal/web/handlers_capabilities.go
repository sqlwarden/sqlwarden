package web

import (
	"net/http"

	"github.com/sqlwarden/internal/response"
)

type EditionFeature struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	DocsURL     *string `json:"docs_url"`
	Navigation  *string `json:"navigation"`
	State       string  `json:"state"`
}

type EditionCapabilities struct {
	Name     string           `json:"edition"`
	Features []EditionFeature `json:"features"`
}

func (app *application) getEditionCapabilities(w http.ResponseWriter, r *http.Request) {
	if err := response.JSON(w, http.StatusOK, app.edition); err != nil {
		app.serverError(w, r, err)
	}
}
