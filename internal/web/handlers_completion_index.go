package web

import (
	"net/http"

	"github.com/sqlwarden/internal/response"
)

func (app *application) getConnectionCompletionIndex(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	out, err := app.queryService().CompletionIndex(r.Context(), p, connectionRef(r), requestSessionID(r))
	if err != nil {
		app.schemaError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}
