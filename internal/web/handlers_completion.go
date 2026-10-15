package web

import (
	"errors"
	"net/http"

	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type completionRequest struct {
	SQL          string `json:"sql"`
	CursorOffset int    `json:"cursor_offset"`
	TriggerKind  string `json:"trigger_kind,omitempty"`
	TriggerChar  string `json:"trigger_character,omitempty"`
}

func (app *application) completeConnectionSQL(w http.ResponseWriter, r *http.Request) {
	p, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, queryapp.MaxCompletionRequestBytes)
	var input completionRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	out, err := app.queryService().Complete(r.Context(), p, connectionRef(r), requestSessionID(r), queryapp.CompleteInput{
		SQL:          input.SQL,
		CursorOffset: input.CursorOffset,
		TriggerKind:  input.TriggerKind,
		TriggerChar:  input.TriggerChar,
	})
	var inputErr *queryapp.CompletionInputError
	switch {
	case err == nil:
	case errors.As(err, &inputErr):
		app.badRequest(w, r, err)
		return
	case errors.Is(err, queryapp.ErrCompletionTooLarge):
		app.errorMessage(w, r, http.StatusRequestEntityTooLarge, "SQL exceeds the 1 MiB completion limit.", nil)
		return
	case errors.Is(err, queryapp.ErrCompletionUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support SQL completion.", nil)
		return
	default:
		app.schemaError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, out); err != nil {
		app.serverError(w, r, err)
	}
}
