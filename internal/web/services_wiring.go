package web

import (
	"errors"
	"net/http"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/exportjob"
	"github.com/sqlwarden/internal/files"
	"github.com/sqlwarden/internal/jobs"
	queryapp "github.com/sqlwarden/internal/query"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) catalogService() *catalog.Service { return app.services.Catalog }

func (app *application) queryService() *queryapp.Service { return app.services.Query }

func (app *application) exportService() *exportjob.Service { return app.services.Exports }

func (app *application) jobService() *jobs.Service { return app.services.Jobs }

func (app *application) workspaceFileService() *files.Service { return app.services.Files }

func (app *application) serviceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, access.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, access.ErrForbidden):
		app.notPermitted(w, r)
	default:
		app.serverError(w, r, err)
	}
}

// queryError maps query service failures to the standard error envelope.
func (app *application) queryError(w http.ResponseWriter, r *http.Request, err error) {
	if !app.queryDomainError(w, r, err) {
		app.serverError(w, r, err)
	}
}

// queryRuntimeError maps query service failures from operations that reach
// the execution runtime; anything that is not a domain failure is a runtime
// failure.
func (app *application) queryRuntimeError(w http.ResponseWriter, r *http.Request, err error) {
	if !app.queryDomainError(w, r, err) {
		app.executionError(w, r, err)
	}
}

func (app *application) queryDomainError(w http.ResponseWriter, r *http.Request, err error) bool {
	var validation *queryapp.ValidationError
	var disabled *queryapp.DisabledError
	var unsafe *queryapp.UnsafeError
	switch {
	case errors.As(err, &validation):
		v := validator.Validator{}
		for field, message := range validation.Fields {
			v.AddFieldError(field, message)
		}
		app.failedValidation(w, r, v)
	case errors.As(err, &disabled):
		v := validator.Validator{}
		v.AddError(disabled.Message)
		app.failedValidation(w, r, v)
	case errors.As(err, &unsafe):
		app.apiError(w, r, http.StatusUnprocessableEntity,
			"unsafe_query_confirmation_required",
			"This statement has no WHERE clause and will affect every row. Confirm to run it anyway.",
			response.APIError{Details: unsafe.Statements},
			nil,
		)
	case errors.Is(err, queryapp.ErrSessionRequired):
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
	case errors.Is(err, queryapp.ErrExplainUnsupported):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "This connection does not support EXPLAIN.", nil)
	case errors.Is(err, queryapp.ErrExplainAnalyzeUnsupported):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "This connection does not support EXPLAIN ANALYZE.", nil)
	case errors.Is(err, queryapp.ErrExplainMultiple):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "EXPLAIN requires exactly one statement.", nil)
	case errors.Is(err, queryapp.ErrExplainAlready):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "This statement is already an EXPLAIN statement.", nil)
	case errors.Is(err, access.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, access.ErrForbidden):
		app.notPermitted(w, r)
	default:
		return false
	}
	return true
}
