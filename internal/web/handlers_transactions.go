package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// transactionStatusView is the JSON shape shared by the transaction endpoints
// and embedded as "transaction" on /query and DDL-apply responses.
type transactionStatusView struct {
	Mode              string   `json:"mode"`
	Open              bool     `json:"open"`
	PendingStatements int      `json:"pending_statements"`
	Statements        []string `json:"statements"`
}

func newTransactionStatusView(status execution.TransactionStatus) transactionStatusView {
	statements := status.Statements
	if statements == nil {
		statements = []string{}
	}
	return transactionStatusView{
		Mode:              string(status.Mode),
		Open:              status.Open,
		PendingStatements: status.PendingStatements,
		Statements:        statements,
	}
}

// resolveTransactionSession resolves the caller's session from the
// X-Warden-Session header, verifying it belongs to this account and
// connection — the same checks executeQuery already performs. Writes an
// error response and returns ok=false on failure.
func (app *application) resolveTransactionSession(w http.ResponseWriter, r *http.Request) (execution.SessionHandle, bool) {
	account := contextGetAccount(r)
	conn := contextGetConnection(r)

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return "", false
	}
	handle := execution.SessionHandle(sessionID)
	session, found, err := app.executionRuntime.Session(r.Context(), handle)
	if err != nil {
		app.serverError(w, r, err)
		return "", false
	}
	if !found {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return "", false
	}
	if session.Scope.AccountID != strconv.FormatInt(account.ID, 10) || session.Scope.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return "", false
	}
	return handle, true
}

func (app *application) setTransactionMode(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	var input struct {
		Mode string              `json:"mode"`
		V    validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	input.V.CheckField(input.Mode == string(execution.TransactionModeAuto) || input.Mode == string(execution.TransactionModeManual),
		"mode", "Mode must be auto or manual.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	status, err := app.executionRuntime.SetTransactionMode(r.Context(), execution.SessionRequest{Handle: session}, execution.TransactionMode(input.Mode))
	if err != nil {
		if errors.Is(err, execution.ErrTransactionOpen) {
			app.errorMessage(w, r, http.StatusConflict, "Commit or roll back the open transaction before switching to auto-commit.", nil)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.logDebug(r, "transaction mode changed", slog.String("mode", input.Mode))
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(status)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) commitTransaction(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	status, err := app.executionRuntime.Commit(r.Context(), execution.SessionRequest{Handle: session})
	if err != nil {
		if errors.Is(err, execution.ErrNoOpenTransaction) {
			app.errorMessage(w, r, http.StatusConflict, "No open transaction to commit.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	app.logDebug(r, "transaction committed")
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(status)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) rollbackTransaction(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	status, err := app.executionRuntime.Rollback(r.Context(), execution.SessionRequest{Handle: session})
	if err != nil {
		if errors.Is(err, execution.ErrNoOpenTransaction) {
			app.errorMessage(w, r, http.StatusConflict, "No open transaction to roll back.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	app.logDebug(r, "transaction rolled back")
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(status)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getTransactionStatus(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	status, err := app.executionRuntime.TransactionStatus(r.Context(), execution.SessionRequest{Handle: session})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(status)); err != nil {
		app.serverError(w, r, err)
	}
}
