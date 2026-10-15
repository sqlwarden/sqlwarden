package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/settings"
)

// authenticateV1 authenticates the Authorization header through the identity
// chain and sets the account, auth session and principal on context.
// Continues without error if no credential is present; use requireAccount to enforce auth.
func (app *application) authenticateV1(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			next.ServeHTTP(w, r)
			return
		}
		runtimeSettings, err := app.services.Settings.EffectiveForOrg(r.Context(), nil)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		r = contextSetRuntimeSettings(r, runtimeSettings)

		result, err := app.authChain.Authenticate(r.Context(), identity.Presented{
			Authorization: authHeader,
			Request:       app.requestAttributes(r),
		})
		var credErr *identity.CredentialError
		if errors.As(err, &credErr) {
			attrs := []slog.Attr{slog.String("reason", credErr.Reason)}
			if credErr.AccountID != 0 {
				attrs = append(attrs, slog.Int64("account_id", credErr.AccountID))
			}
			if credErr.CredentialID != "" {
				attrs = append(attrs, slog.String("auth_session_id", credErr.CredentialID))
			}
			app.logWarn(r, "authentication rejected", attrs...)
			app.rejectCredential(w, r, credErr.Reason)
			return
		}
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if result.AuthSession != nil {
			r = contextSetAuthSession(r, *result.AuthSession)
		}
		r = contextSetAccount(r, result.Account)
		r = contextSetPrincipal(r, result.Principal)
		next.ServeHTTP(w, r)
	})
}

func revocationFromContext(ctx context.Context) bool {
	effective, ok := ctx.Value(runtimeSettingsKey).(settings.Effective)
	return ok && effective.SessionsRevocationEnabled
}

// requireAccount rejects the request with 401 if no authenticated account is in context.
func (app *application) requireAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account := contextGetAccount(r)
		if account.ID == 0 {
			app.authenticationRequired(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// orgScope enters the organization named by the URL slug and records its id
// in the path ids. Entry is the organization membership gate: an unknown slug
// is 404 and a non-member is 403. Requires requireAccount to have run first.
func (app *application) orgScope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := app.requestPrincipal(w, r)
		if !ok {
			return
		}
		runtimeSettings, ok := contextGetRuntimeSettings(r)
		if !ok {
			var err error
			runtimeSettings, err = app.services.Settings.EffectiveForOrg(r.Context(), nil)
			if err != nil {
				app.serverError(w, r, err)
				return
			}
		}
		authSession := contextGetAuthSession(r)
		entry, err := app.services.Identity.EnterOrg(r.Context(), p, identity.EnterOrgInput{
			Slug:                 chi.URLParam(r, "org_slug"),
			RevocationEnabled:    runtimeSettings.SessionsRevocationEnabled,
			AuthSessionID:        authSession.ID,
			AuthSessionExpiresAt: authSession.ExpiresAt,
		})
		var rejection *identity.OrgAccessRejection
		switch {
		case err == nil:
		case errors.As(err, &rejection):
			app.logWarn(r, "organization access session rejected",
				slog.Int64("account_id", rejection.AccountID),
				slog.Int64("org_id", rejection.OrgID),
				slog.String("auth_session_id", rejection.AuthSessionID),
				slog.String("reason", rejection.Reason))
			app.notPermitted(w, r)
			return
		case errors.Is(err, identity.ErrAuthSessionRequired):
			app.invalidAuthenticationToken(w, r)
			return
		case errors.Is(err, identity.ErrNotFound):
			app.notFound(w, r)
			return
		case errors.Is(err, identity.ErrForbidden):
			app.notPermitted(w, r)
			return
		default:
			app.serverError(w, r, err)
			return
		}
		ids := contextGetPathIDs(r)
		ids.OrgID, ids.OrgSlug = entry.ID, entry.Slug
		next.ServeHTTP(w, contextSetPathIDs(r, ids))
	})
}

// pathID parses a positive integer URL parameter into the path ids. It reads
// no store: services decide whether the id exists and belongs to its parent.
// A malformed id cannot name any resource, so it is 404 like a missing one.
func (app *application) pathID(param string, set func(*pathIDs, int64)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(chi.URLParam(r, param), 10, 64)
			if err != nil || id <= 0 {
				app.notFound(w, r)
				return
			}
			ids := contextGetPathIDs(r)
			set(&ids, id)
			next.ServeHTTP(w, contextSetPathIDs(r, ids))
		})
	}
}

func (app *application) workspacePath(next http.Handler) http.Handler {
	return app.pathID("ws_id", func(ids *pathIDs, id int64) { ids.WorkspaceID = id })(next)
}

func (app *application) environmentPath(next http.Handler) http.Handler {
	return app.pathID("env_id", func(ids *pathIDs, id int64) { ids.EnvironmentID = id })(next)
}

func (app *application) connectionPath(next http.Handler) http.Handler {
	return app.pathID("conn_id", func(ids *pathIDs, id int64) { ids.ConnectionID = id })(next)
}
