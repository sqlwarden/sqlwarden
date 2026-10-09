package web

import (
	"errors"
	"log/slog"
	"mime"
	"net/http"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/exportjob"
	"github.com/sqlwarden/internal/exports"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
)

type exportRequest struct {
	SQL      string `json:"sql"`
	Format   string `json:"format"`
	Filename string `json:"filename"`
}

func (r exportRequest) input() exportjob.RequestInput {
	return exportjob.RequestInput{SQL: r.SQL, Format: r.Format, Filename: r.Filename}
}

func (app *application) createConnectionExport(w http.ResponseWriter, r *http.Request) {
	var input exportRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ref := connectionRef(r)
	job, err := app.exportService().Request(r.Context(), principal, ref, input.input())
	if err != nil {
		app.exportError(w, r, err)
		return
	}
	app.logInfo(r, "export job queued", slog.String("job.id", job.ID), slog.Int64("connection_id", ref.ConnectionID), slog.String("format", exportjob.NormalizedFormat(input.Format)))
	if err := response.JSON(w, http.StatusCreated, job); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) downloadConnectionExport(w http.ResponseWriter, r *http.Request) {
	var input exportRequest
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	ref := connectionRef(r)
	sessionID := requestSessionID(r)
	started := false
	result, err := app.exportService().Download(r.Context(), principal, ref, sessionID, input.input(), w, func(filename string) {
		started = true
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
		w.WriteHeader(http.StatusOK)
	})
	if err != nil {
		if !started {
			app.exportError(w, r, err)
			return
		}
		app.logWarn(r, "synchronous export failed", slog.Int64("connection_id", ref.ConnectionID), slog.String("session_id", string(sessionID)), slog.String("error", exportjob.ErrorCategory(err)))
		if errors.Is(err, exports.ErrByteLimitExceeded) {
			panic(http.ErrAbortHandler)
		}
		return
	}
	app.logInfo(r, "synchronous export completed", slog.Int64("connection_id", ref.ConnectionID), slog.String("session_id", string(sessionID)), slog.Int64("rows", result.Rows), slog.Int64("bytes", result.Bytes))
}

// exportError maps export service failures to the standard error envelope.
// Failures that are not export domain or authorization errors come from the
// execution runtime.
func (app *application) exportError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *exportjob.ValidationError
	switch {
	case errors.As(err, &validation):
		app.failedValidation(w, r, fieldErrors(validation.Fields))
	case errors.Is(err, exportjob.ErrClassifierUnavailable):
		app.errorMessage(w, r, http.StatusNotImplemented, exportjob.ClassifierUnavailableMessage, nil)
	case errors.Is(err, exportjob.ErrSessionRequired):
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
	case errors.Is(err, access.ErrNotFound), errors.Is(err, access.ErrForbidden):
		app.serviceError(w, r, err)
	default:
		app.executionError(w, r, err)
	}
}
