package web

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listWorkspaceJobs(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}

	result, err := app.jobService().List(r.Context(), principal, contextGetOrg(r).ID, contextGetWorkspace(r).ID, q.Page, q.PageSize)
	if err != nil {
		app.serviceError(w, r, err)
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
	job, err := app.jobService().Get(r.Context(), principal, contextGetOrg(r).ID, contextGetWorkspace(r).ID, chi.URLParam(r, "job_id"))
	if err != nil {
		app.serviceError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, job); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listWorkspaceJobEvents(w http.ResponseWriter, r *http.Request) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	events, err := app.jobService().Events(
		r.Context(),
		principal,
		contextGetOrg(r).ID,
		contextGetWorkspace(r).ID,
		chi.URLParam(r, "job_id"),
		r.URL.Query().Get("after_id"),
		q.PageSize,
	)
	if err != nil {
		app.serviceError(w, r, err)
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
	job, err := app.jobService().RequestCancel(r.Context(), principal, contextGetOrg(r).ID, contextGetWorkspace(r).ID, chi.URLParam(r, "job_id"))
	if err != nil {
		app.serviceError(w, r, err)
		return
	}
	app.logInfo(r, "job cancellation requested", slog.String("job.id", job.ID), slog.String("job.type", job.Type), slog.String("job.status", job.Status))
	if err := response.JSON(w, http.StatusOK, job); err != nil {
		app.serverError(w, r, err)
	}
}
