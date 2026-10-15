package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/response"
)

func (app *application) jobError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *jobs.ValidationError
	if errors.As(err, &validation) {
		app.failedValidation(w, r, fieldErrors(validation.Fields))
		return
	}
	app.serviceError(w, r, err)
}

func (app *application) listWorkspaceJobs(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	result, err := app.jobService().List(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, query.Get("page"), query.Get("page_size"))
	if err != nil {
		app.jobError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspaceJob(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	job, err := app.jobService().Get(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, chi.URLParam(r, "job_id"))
	if err != nil {
		app.serviceError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, job); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listWorkspaceJobEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	events, err := app.jobService().Events(
		r.Context(),
		principal,
		contextGetPathIDs(r).OrgID,
		contextGetPathIDs(r).WorkspaceID,
		chi.URLParam(r, "job_id"),
		query.Get("after_id"),
		query.Get("page_size"),
	)
	if err != nil {
		app.jobError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, events); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) cancelWorkspaceJob(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	job, err := app.jobService().RequestCancel(r.Context(), principal, contextGetPathIDs(r).OrgID, contextGetPathIDs(r).WorkspaceID, chi.URLParam(r, "job_id"))
	if err != nil {
		app.serviceError(w, r, err)
		return
	}
	app.logInfo(r, "job cancellation requested", slog.String("job.id", job.ID), slog.String("job.type", job.Type), slog.String("job.status", job.Status))
	if err := response.JSON(w, http.StatusOK, job); err != nil {
		app.serverError(w, r, err)
	}
}
