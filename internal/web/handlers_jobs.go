package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/jobs"
	"github.com/sqlwarden/internal/response"
)

func (app *application) listWorkspaceJobs(w http.ResponseWriter, r *http.Request) {
	if !app.ensureWorkspaceJobAccess(w, r) {
		return
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	result, err := app.workspaceJobStore().ListUserWorkspaceJobs(r.Context(), org.ID, ws.ID, account.ID, q.Page, q.PageSize)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getWorkspaceJob(w http.ResponseWriter, r *http.Request) {
	if !app.ensureWorkspaceJobAccess(w, r) {
		return
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	job, found, err := app.workspaceJobStore().GetUserWorkspaceJob(r.Context(), org.ID, ws.ID, account.ID, chi.URLParam(r, "job_id"))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if err := response.JSON(w, http.StatusOK, job); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) listWorkspaceJobEvents(w http.ResponseWriter, r *http.Request) {
	if !app.ensureWorkspaceJobAccess(w, r) {
		return
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	events, err := app.workspaceJobStore().ListUserWorkspaceJobEvents(
		r.Context(),
		org.ID,
		ws.ID,
		account.ID,
		chi.URLParam(r, "job_id"),
		r.URL.Query().Get("after_id"),
		q.PageSize,
	)
	if errors.Is(err, jobs.ErrNotFound) {
		app.notFound(w, r)
		return
	}
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, events); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) cancelWorkspaceJob(w http.ResponseWriter, r *http.Request) {
	if !app.ensureWorkspaceJobAccess(w, r) {
		return
	}
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	job, found, err := app.workspaceJobStore().RequestCancelUserWorkspaceJob(r.Context(), org.ID, ws.ID, account.ID, chi.URLParam(r, "job_id"))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	app.logInfo(r, "job cancellation requested", slog.String("job.id", job.ID), slog.String("job.type", job.Type), slog.String("job.status", job.Status))
	if err := response.JSON(w, http.StatusOK, job); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) ensureWorkspaceJobAccess(w http.ResponseWriter, r *http.Request) bool {
	account := contextGetAccount(r)
	org := contextGetOrg(r)
	ws := contextGetWorkspace(r)
	ok, err := app.db.IsEffectiveWorkspaceMember(r.Context(), org.ID, ws.ID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return false
	}
	if !ok {
		app.notPermitted(w, r)
		return false
	}
	return true
}

func (app *application) workspaceJobStore() *jobs.Store {
	if app.jobStore == nil {
		app.jobStore = jobs.NewStore(app.db)
	}
	return app.jobStore
}
