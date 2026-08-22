package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/engine/transaction"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// transactionStatusView is the JSON shape shared by the transaction endpoints
// and embedded as "transaction" on /query and DDL-apply responses.
type transactionStatusView struct {
	Mode              string `json:"mode"`
	Open              bool   `json:"open"`
	PendingStatements int    `json:"pending_statements"`
}

func newTransactionStatusView(status connection.TransactionStatus) transactionStatusView {
	return transactionStatusView{
		Mode:              string(status.Mode),
		Open:              status.Open,
		PendingStatements: status.PendingStatements,
	}
}

// resolveTransactionSession resolves the caller's session from the
// X-Warden-Session header, verifying it belongs to this account and
// connection — the same checks executeQuery already performs. Writes an
// error response and returns ok=false on failure.
func (app *application) resolveTransactionSession(w http.ResponseWriter, r *http.Request) (*connection.Session, bool) {
	account := contextGetAccount(r)
	conn := contextGetConnection(r)

	sessionID := r.Header.Get("X-Warden-Session")
	if sessionID == "" {
		app.errorMessage(w, r, http.StatusBadRequest, "X-Warden-Session header is required.", nil)
		return nil, false
	}
	session, ok := app.connManager.Get(sessionID)
	if !ok {
		app.errorMessage(w, r, http.StatusGone, "Session has expired or does not exist.", nil)
		return nil, false
	}
	if session.AccountID != strconv.FormatInt(account.ID, 10) || session.ConnectionID != strconv.FormatInt(conn.ID, 10) {
		app.notPermitted(w, r)
		return nil, false
	}
	return session, true
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
	input.V.CheckField(input.Mode == string(connection.TxModeAuto) || input.Mode == string(connection.TxModeManual),
		"mode", "Mode must be auto or manual.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	if err := session.SetTransactionMode(r.Context(), connection.TxMode(input.Mode)); err != nil {
		if errors.Is(err, connection.ErrTransactionOpen) {
			app.errorMessage(w, r, http.StatusConflict, "Commit or roll back the open transaction before switching to auto-commit.", nil)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "transaction mode changed", slog.String("session_id", session.ID), slog.String("mode", input.Mode))
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(session.TransactionStatus())); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) commitTransaction(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	if err := session.CommitTransaction(r.Context()); err != nil {
		if errors.Is(err, transaction.ErrNoOpenTransaction) {
			app.errorMessage(w, r, http.StatusConflict, "No open transaction to commit.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	app.logInfo(r, "transaction committed", slog.String("session_id", session.ID))
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(session.TransactionStatus())); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) rollbackTransaction(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	if err := session.RollbackTransaction(r.Context()); err != nil {
		if errors.Is(err, transaction.ErrNoOpenTransaction) {
			app.errorMessage(w, r, http.StatusConflict, "No open transaction to roll back.", nil)
			return
		}
		app.errorMessage(w, r, http.StatusUnprocessableEntity, err.Error(), nil)
		return
	}
	app.logInfo(r, "transaction rolled back", slog.String("session_id", session.ID))
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(session.TransactionStatus())); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) getTransactionStatus(w http.ResponseWriter, r *http.Request) {
	session, ok := app.resolveTransactionSession(w, r)
	if !ok {
		return
	}
	if err := response.JSON(w, http.StatusOK, newTransactionStatusView(session.TransactionStatus())); err != nil {
		app.serverError(w, r, err)
	}
}
