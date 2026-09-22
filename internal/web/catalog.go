package web

import (
	"errors"
	"net/http"

	"github.com/sqlwarden/internal/catalog"
	"github.com/sqlwarden/internal/validator"
)

// catalogService returns the catalog application service supplied by the
// composition root.
func (app *application) catalogService() *catalog.Service { return app.catalog }

// catalogActor identifies the account and organization a catalog use case runs
// for. Resolving it is transport work: the catalog is given a principal, it
// does not read one out of a request.
func catalogActor(r *http.Request) catalog.Actor {
	return catalog.Actor{AccountID: contextGetAccount(r).ID, OrgID: contextGetOrg(r).ID}
}

// catalogError maps a catalog domain error onto the HTTP error envelope. It is
// the only place the transport decides what a catalog refusal looks like over
// HTTP; callers that need a resource-specific duplicate-name message handle
// that case before delegating here.
func (app *application) catalogError(w http.ResponseWriter, r *http.Request, err error) {
	var invalid *catalog.ValidationError
	if errors.As(err, &invalid) {
		app.failedValidation(w, r, invalid.Validator)
		return
	}

	var activeSessions *catalog.ActiveSessionsError
	if errors.As(err, &activeSessions) {
		app.errorMessage(w, r, http.StatusConflict, activeSessionsMessage(activeSessions.Reason), nil)
		return
	}

	var sealErr *catalog.SealError
	if errors.As(err, &sealErr) {
		app.errorMessage(w, r, http.StatusUnprocessableEntity, sealErr.Error(), nil)
		return
	}

	switch {
	case errors.Is(err, catalog.ErrNotFound):
		app.notFound(w, r)
	case errors.Is(err, catalog.ErrCredentialsMasked):
		app.notPermitted(w, r)
	case errors.Is(err, catalog.ErrNameRequired):
		v := validator.Validator{}
		v.AddFieldError("name", "Name is required.")
		app.failedValidation(w, r, v)
	case errors.Is(err, catalog.ErrLastOwner):
		v := validator.Validator{}
		v.AddError("Cannot remove the last owner of an organization.")
		app.failedValidation(w, r, v)
	case errors.Is(err, catalog.ErrInvalidRole):
		v := validator.Validator{}
		v.AddFieldError("role", "Role must be Owner, Administrator, or Baseline Access.")
		app.failedValidation(w, r, v)
	case errors.Is(err, catalog.ErrEnvironmentHasConnections):
		app.errorMessage(w, r, http.StatusUnprocessableEntity, "Environment has connections.", nil)
	default:
		app.serverError(w, r, err)
	}
}

func activeSessionsMessage(reason catalog.ActiveSessionsReason) string {
	if reason == catalog.ActiveSessionsDSNRotation {
		return "Connection has active sessions. Retry with force=true to rotate the DSN and drop them."
	}
	return "Connection has active sessions. Retry with force=true to change its default scope and drop them."
}
