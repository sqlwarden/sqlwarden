package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/execution"
)

const sessionHeader = "X-Warden-Session"

// runtimeScope is the execution scope of the request's account on the routed
// connection. Every runtime call re-checks it against the session it names.
func runtimeScope(r *http.Request) execution.Scope {
	return execution.Scope{
		OrgID:        strconv.FormatInt(contextGetOrg(r).ID, 10),
		WorkspaceID:  strconv.FormatInt(contextGetWorkspace(r).ID, 10),
		AccountID:    strconv.FormatInt(contextGetAccount(r).ID, 10),
		ConnectionID: strconv.FormatInt(contextGetConnection(r).ID, 10),
	}
}

func requestSessionID(r *http.Request) execution.SessionID {
	return execution.SessionID(r.Header.Get(sessionHeader))
}

// requireSessionID reads the session header, answering 400 when it is absent.
func (app *application) requireSessionID(w http.ResponseWriter, r *http.Request) (execution.SessionID, bool) {
	id := requestSessionID(r)
	if id == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return "", false
	}
	return id, true
}

// resolveRuntimeSession requires the session header and confirms the session
// is live and owned by the request scope. An unknown session and one owned by
// another scope both answer 410 so neither can be told apart.
func (app *application) resolveRuntimeSession(w http.ResponseWriter, r *http.Request) (execution.SessionInfo, bool) {
	id, ok := app.requireSessionID(w, r)
	if !ok {
		return execution.SessionInfo{}, false
	}
	info, err := app.runtime.Get(r.Context(), runtimeScope(r), id)
	if err != nil {
		app.executionError(w, r, err)
		return execution.SessionInfo{}, false
	}
	return info, true
}

func isRequestCanceled(r *http.Request, err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || r.Context().Err() != nil
}

// executionError writes the response for a runtime error. Lost sessions and
// transactions answer 410 because the client must reconnect; a cancelled or
// interrupted statement answers 499; an error the target itself returned
// answers 422 with the target's message. Anything unclassified is internal
// and answers 500 without echoing its text.
func (app *application) executionError(w http.ResponseWriter, r *http.Request, err error) {
	var failure *execution.Failure
	if errors.As(err, &failure) {
		switch failure.Code {
		case execution.FailureSessionNotFound, execution.FailureSessionLost, execution.FailureTransactionLost:
			app.logDebug(r, "execution session unavailable", slog.String("failure_code", string(failure.Code)))
			app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
			return
		case execution.FailureCursorLost:
			app.logDebug(r, "query cursor unavailable", slog.String("failure_code", string(failure.Code)))
			app.queryCursorUnavailable(w, r)
			return
		case execution.FailureExecutionOutcomeUnknown:
			app.logDebug(r, "execution cancelled with unknown outcome", slog.String("failure_code", string(failure.Code)))
			app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
			return
		case execution.FailureLimitExceeded:
			app.logInfo(r, "execution limit exceeded", slog.String("failure_code", string(failure.Code)))
			app.errorMessage(w, r, http.StatusUnprocessableEntity, "The result exceeded the configured size limit.", nil)
			return
		}
	}
	if isRequestCanceled(r, err) {
		app.errorMessage(w, r, statusClientClosedRequest, "Query was cancelled.", nil)
		return
	}
	var target *execution.TargetError
	switch {
	case errors.As(err, &target):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, target.Error(), nil)
	case errors.Is(err, execution.ErrCursorsUnsupported):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "Connection driver does not support query cursors.", nil)
	case errors.Is(err, execution.ErrTransactionOpen):
		app.errorMessage(w, r, http.StatusConflict, "Commit or roll back the open transaction first.", nil)
	case errors.Is(err, execution.ErrNoOpenTransaction):
		app.errorMessage(w, r, http.StatusConflict, "No open transaction.", nil)
	case errors.Is(err, execution.ErrDDLUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support structured DDL.", nil)
	case errors.Is(err, execution.ErrSchemaUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema inspection.", nil)
	case errors.Is(err, execution.ErrRelationshipsUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support schema relationships.", nil)
	case errors.Is(err, execution.ErrDefinitionUnsupported):
		app.errorMessage(w, r, http.StatusNotImplemented, "This driver does not support on-demand object definitions.", nil)
	default:
		app.serverError(w, r, err)
	}
}
