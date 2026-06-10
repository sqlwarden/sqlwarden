package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
	"github.com/uptrace/bun/driver/pgdriver"
)

const statusClientClosedRequest = 499

const (
	apiErrorBadRequest                 = "bad_request"
	apiErrorAuthenticationRequired     = "authentication_required"
	apiErrorInvalidAuthenticationToken = "invalid_authentication_token"
	apiErrorNotPermitted               = "not_permitted"
	apiErrorNotFound                   = "not_found"
	apiErrorMethodNotAllowed           = "method_not_allowed"
	apiErrorValidationFailed           = "validation_failed"
	apiErrorConflict                   = "conflict"
	apiErrorResourceInUse              = "resource_in_use"
	apiErrorInternalServer             = "internal_server_error"
)

func (app *application) reportServerError(r *http.Request, err error) {
	var (
		message = err.Error()
		method  = r.Method
		url     = r.URL.String()
		trace   = string(debug.Stack())
	)

	requestAttrs := slog.Group("request", "method", method, "url", url)
	app.logger.Error(message, requestAttrs, "trace", trace)

	if app.config.Notifications.Email != "" {
		data := app.newEmailData()
		data["Message"] = message
		data["RequestMethod"] = method
		data["RequestURL"] = url
		data["Trace"] = trace

		err := app.mailer.Send(app.config.Notifications.Email, data, "error-notification.tmpl")
		if err != nil {
			trace = string(debug.Stack())
			app.logger.Error(err.Error(), requestAttrs, "trace", trace)
		}
	}
}

func (app *application) errorMessage(w http.ResponseWriter, r *http.Request, status int, message string, headers http.Header) {
	app.apiError(w, r, status, defaultAPIErrorCode(status), message, response.APIError{}, headers)
}

func (app *application) apiError(w http.ResponseWriter, r *http.Request, status int, code, message string, apiErr response.APIError, headers http.Header) {
	apiErr.Code = code
	apiErr.Message = message
	err := response.JSONWithHeaders(w, status, response.APIErrorEnvelope{Error: apiErr}, headers)
	if err != nil {
		app.reportServerError(r, err)
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.reportServerError(r, err)

	message := "The server encountered a problem and could not process your request."
	app.errorMessage(w, r, http.StatusInternalServerError, message, nil)
}

func (app *application) notFound(w http.ResponseWriter, r *http.Request) {
	message := "The requested resource could not be found."
	app.errorMessage(w, r, http.StatusNotFound, message, nil)
}

func (app *application) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	message := fmt.Sprintf("The %s method is not supported for this resource.", r.Method)
	app.errorMessage(w, r, http.StatusMethodNotAllowed, message, nil)
}

func (app *application) badRequest(w http.ResponseWriter, r *http.Request, err error) {
	app.errorMessage(w, r, http.StatusBadRequest, err.Error(), nil)
}

func (app *application) failedValidation(w http.ResponseWriter, r *http.Request, v validator.Validator) {
	app.apiError(w, r, http.StatusUnprocessableEntity, apiErrorValidationFailed, firstValidationMessage(v), response.APIError{
		FieldErrors: v.FieldErrors,
		Errors:      v.Errors,
	}, nil)
}

func (app *application) failedDuplicateField(w http.ResponseWriter, r *http.Request, field, message string) {
	v := validator.Validator{}
	v.AddFieldError(field, message)
	app.failedValidation(w, r, v)
}

func (app *application) invalidAuthenticationToken(w http.ResponseWriter, r *http.Request) {
	headers := make(http.Header)
	headers.Set("WWW-Authenticate", "Bearer")

	app.apiError(w, r, http.StatusUnauthorized, apiErrorInvalidAuthenticationToken, "Invalid authentication token.", response.APIError{}, headers)
}

func (app *application) notPermitted(w http.ResponseWriter, r *http.Request) {
	message := "You do not have permission to perform this action."
	app.apiError(w, r, http.StatusForbidden, apiErrorNotPermitted, message, response.APIError{}, nil)
}

// isUniqueViolation returns true if err is a unique-constraint violation from
// either the PostgreSQL (pgx) or SQLite driver.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// isForeignKeyViolation returns true if err is a foreign-key constraint violation
// from either the PostgreSQL (pgx) or SQLite driver.
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		return pgErr.Field('C') == "23503"
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}

func (app *application) authenticationRequired(w http.ResponseWriter, r *http.Request) {
	headers := make(http.Header)
	headers.Set("WWW-Authenticate", "Bearer")

	app.apiError(w, r, http.StatusUnauthorized, apiErrorAuthenticationRequired, "You must be authenticated to access this resource.", response.APIError{}, headers)
}

func firstValidationMessage(v validator.Validator) string {
	for _, message := range v.FieldErrors {
		if strings.TrimSpace(message) != "" {
			return message
		}
	}
	for _, message := range v.Errors {
		if strings.TrimSpace(message) != "" {
			return message
		}
	}
	return "Request validation failed."
}

func defaultAPIErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return apiErrorBadRequest
	case http.StatusUnauthorized:
		return apiErrorAuthenticationRequired
	case http.StatusForbidden:
		return apiErrorNotPermitted
	case http.StatusNotFound:
		return apiErrorNotFound
	case http.StatusMethodNotAllowed:
		return apiErrorMethodNotAllowed
	case http.StatusUnprocessableEntity:
		return apiErrorValidationFailed
	case http.StatusConflict:
		return apiErrorConflict
	case http.StatusInternalServerError:
		return apiErrorInternalServer
	default:
		return fmt.Sprintf("http_%d", status)
	}
}
