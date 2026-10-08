package web

import (
	"errors"
	"log/slog"
	"net/http"

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

func newTransactionStatusView(status execution.TxStatus) transactionStatusView {
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

func (app *application) writeTransactionStatus(w http.ResponseWriter, r *http.Request, status execution.TxStatus) {
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(status)); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) setTransactionMode(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveRuntimeSession(w, r)
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
	input.V.CheckField(input.Mode == string(execution.TxModeAuto) || input.Mode == string(execution.TxModeManual),
		"mode", "Mode must be auto or manual.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	status, err := app.runtime.SetMode(r.Context(), runtimeScope(r), session.ID, execution.TxMode(input.Mode))
	if err != nil {
		var failure *execution.Failure
		switch {
		case errors.Is(err, execution.ErrTransactionOpen):
			app.errorMessage(w, r, http.StatusConflict, "Commit or roll back the open transaction before switching to auto-commit.", nil)
		case errors.As(err, &failure):
			app.executionError(w, r, err)
		default:
			app.serverError(w, r, err)
		}
		return
	}
	app.logInfo(r, "transaction mode changed", slog.String("session_id", string(session.ID)), slog.String("mode", input.Mode))
	app.writeTransactionStatus(w, r, status)
}

func (app *application) commitTransaction(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := app.requireSessionID(w, r)
	if !ok {
		return
	}
	status, err := app.runtime.Commit(r.Context(), runtimeScope(r), sessionID)
	if err != nil {
		if errors.Is(err, execution.ErrNoOpenTransaction) {
			app.errorMessage(w, r, http.StatusConflict, "No open transaction to commit.", nil)
			return
		}
		app.executionError(w, r, err)
		return
	}
	app.logInfo(r, "transaction committed", slog.String("session_id", string(sessionID)))
	app.writeTransactionStatus(w, r, status)
}

func (app *application) rollbackTransaction(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := app.requireSessionID(w, r)
	if !ok {
		return
	}
	status, err := app.runtime.Rollback(r.Context(), runtimeScope(r), sessionID)
	if err != nil {
		if errors.Is(err, execution.ErrNoOpenTransaction) {
			app.errorMessage(w, r, http.StatusConflict, "No open transaction to roll back.", nil)
			return
		}
		app.executionError(w, r, err)
		return
	}
	app.logInfo(r, "transaction rolled back", slog.String("session_id", string(sessionID)))
	app.writeTransactionStatus(w, r, status)
}

func (app *application) getTransactionStatus(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := app.requireSessionID(w, r)
	if !ok {
		return
	}
	status, err := app.runtime.Status(r.Context(), runtimeScope(r), sessionID)
	if err != nil {
		app.executionError(w, r, err)
		return
	}
	app.writeTransactionStatus(w, r, status)
}
