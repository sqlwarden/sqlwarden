package web

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/identity"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/validator"
)

// identityError translates identity service failures to the standard error
// envelope.
func (app *application) identityError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *identity.ValidationError
	var credErr *identity.CredentialError
	switch {
	case errors.As(err, &validation):
		app.failedValidation(w, r, fieldErrors(validation.Fields))
	case errors.As(err, &credErr):
		app.rejectCredential(w, r, credErr.Reason)
	case errors.Is(err, identity.ErrSetupIncomplete):
		app.errorMessage(w, r, http.StatusForbidden, "Instance setup is not complete.", nil)
	case errors.Is(err, identity.ErrSignInUnavailable):
		app.notFound(w, r)
	case errors.Is(err, identity.ErrAlreadyConfigured):
		app.errorMessage(w, r, http.StatusConflict, "Instance is already configured.", nil)
	case errors.Is(err, identity.ErrLastInstanceAdmin):
		v := validator.Validator{}
		v.AddError("Cannot remove the last instance admin.")
		app.failedValidation(w, r, v)
	case errors.Is(err, identity.ErrInvalidRefreshToken):
		app.invalidAuthenticationToken(w, r)
	default:
		app.serviceError(w, r, err)
	}
}

func (app *application) setRefreshCookie(w http.ResponseWriter, r *http.Request, issued identity.Issued) {
	// Local HTTP is supported; refreshTokenCookie enforces Secure for HTTPS and built-in TLS.
	http.SetCookie(w, app.refreshTokenCookie(r, issued.RefreshToken, int(issued.RefreshTTL.Seconds())))
}

func (app *application) registerAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	account, err := app.services.Identity.Register(r.Context(), identity.RegisterInput{
		Email: input.Email, Name: input.Name, Password: input.Password,
	})
	if err != nil {
		app.identityError(w, r, err)
		return
	}

	app.logInfo(r, "account registered", slog.Int64("account_id", account.ID))
	if err := response.JSON(w, http.StatusCreated, account); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) loginAccount(w http.ResponseWriter, r *http.Request) {
	if !app.services.Identity.SignInEnabled() {
		app.notFound(w, r)
		return
	}

	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	issued, err := app.services.Identity.Login(r.Context(), identity.LoginInput{
		Email: input.Email, Password: input.Password,
	}, app.requestAttributes(r))
	if err != nil {
		app.identityError(w, r, err)
		return
	}

	app.setRefreshCookie(w, r, issued)
	app.logInfo(r, "account logged in", slog.Int64("account_id", issued.AccountID), slog.String("auth_session_id", issued.AuthSessionID))
	if err := response.JSON(w, http.StatusOK, map[string]string{"access_token": issued.AccessToken}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshToken(w http.ResponseWriter, r *http.Request) {
	var raw string
	if cookie, err := r.Cookie("refresh_token"); err == nil {
		raw = cookie.Value
	}

	issued, err := app.services.Identity.Refresh(r.Context(), identity.RefreshInput{RefreshToken: raw}, app.requestAttributes(r))
	var rejection *identity.RefreshRejection
	if errors.As(err, &rejection) {
		app.logRefreshRejection(r, rejection)
	}
	if err != nil {
		app.identityError(w, r, err)
		return
	}

	app.logInfo(r, "access token refreshed", slog.Int64("account_id", issued.AccountID), slog.String("auth_session_id", issued.AuthSessionID))
	app.setRefreshCookie(w, r, issued)
	if err := response.JSON(w, http.StatusOK, map[string]string{"access_token": issued.AccessToken}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) logRefreshRejection(r *http.Request, rejection *identity.RefreshRejection) {
	var msg string
	switch {
	case strings.HasPrefix(rejection.Reason, "refresh_token_"):
		msg = "refresh token rejected"
	case strings.HasPrefix(rejection.Reason, "auth_session_"):
		msg = "refresh auth session rejected"
	default:
		return
	}
	app.logWarn(r, msg, slog.Int64("account_id", rejection.AccountID), slog.String("auth_session_id", rejection.AuthSessionID), slog.String("reason", rejection.Reason))
}

func (app *application) logoutAccount(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("refresh_token"); err == nil {
		// Logout always clears the cookie; a failed revocation is logged, not
		// surfaced, so the client is never left holding the token.
		out, err := app.services.Identity.Logout(r.Context(), identity.LogoutInput{RefreshToken: cookie.Value})
		if err != nil {
			app.logWarn(r, "logout revocation failed", slog.Any("error", err))
		} else if out.AuthSessionID != "" {
			app.logInfo(r, "account logged out", slog.Int64("account_id", out.AccountID), slog.String("auth_session_id", out.AuthSessionID))
		}
	}
	// Local HTTP is supported; refreshTokenCookie enforces Secure for HTTPS and built-in TLS.
	http.SetCookie(w, app.refreshTokenCookie(r, "", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) refreshTokenCookie(r *http.Request, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     "refresh_token",
		Value:    value,
		HttpOnly: true,
		Secure:   app.secureCookies(r),
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   maxAge,
	}
}

func (app *application) secureCookies(r *http.Request) bool {
	if app.config.TLS.Enabled || r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	settings, err := app.services.Settings.Instance(r.Context())
	if err != nil {
		// Authentication settings are database-owned. If they are unavailable,
		// prefer a secure cookie over falling back to stale node-local configuration.
		return true
	}
	baseURL, err := url.Parse(settings.BaseURL)
	return err == nil && strings.EqualFold(baseURL.Scheme, "https")
}

// readSessionPage reads the page of a session list query.
func (app *application) readSessionPage(w http.ResponseWriter, r *http.Request) (identity.Page, bool) {
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return identity.Page{}, false
	}
	return identity.Page{Page: q.Page, PageSize: q.PageSize}, true
}

func (app *application) listAccountSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	page, ok := app.readSessionPage(w, r)
	if !ok {
		return
	}
	sessions, err := app.services.Identity.ListOwnSessions(r.Context(), principal, page)
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, sessions); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeAccountSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if err := app.services.Identity.RevokeOwnSession(r.Context(), principal, sessionID); err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "auth session revoked", slog.Int64("target_account_id", principal.Subject.ID), slog.String("auth_session_id", sessionID), slog.String("reason", "user_revoked"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeAccountSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	if err := app.services.Identity.RevokeOwnSessions(r.Context(), principal); err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "auth sessions revoked", slog.Int64("target_account_id", principal.Subject.ID), slog.String("reason", "user_revoked_all"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listInstanceAccountSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	page, ok := app.readSessionPage(w, r)
	if !ok {
		return
	}
	sessions, err := app.services.Identity.ListInstanceSessions(r.Context(), principal, accountID, page)
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, sessions); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeInstanceAccountSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if err := app.services.Identity.RevokeInstanceSession(r.Context(), principal, accountID, sessionID); err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "auth session revoked", slog.Int64("target_account_id", accountID), slog.String("auth_session_id", sessionID), slog.String("reason", "instance_admin_revoked"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeInstanceAccountSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	if err := app.services.Identity.RevokeInstanceSessions(r.Context(), principal, accountID); err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "auth sessions revoked", slog.Int64("target_account_id", accountID), slog.String("reason", "instance_admin_revoked_all"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listOrgMemberAccessSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	page, ok := app.readSessionPage(w, r)
	if !ok {
		return
	}
	sessions, err := app.services.Identity.ListOrgMemberAccessSessions(r.Context(), principal, contextGetPathIDs(r).OrgID, accountID, page)
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, sessions); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeOrgMemberAccessSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if err := app.services.Identity.RevokeOrgMemberAccessSession(r.Context(), principal, contextGetPathIDs(r).OrgID, accountID, sessionID); err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "org access session revoked", slog.Int64("target_account_id", accountID), slog.String("org_access_session_id", sessionID), slog.String("reason", "org_admin_revoked"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getAccount(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	account, err := app.services.Identity.GetAccount(r.Context(), principal)
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, account); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	account, err := app.services.Identity.UpdateAccount(r.Context(), principal, identity.UpdateAccountInput{Name: input.Name})
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "account updated", slog.Int64("account_id", account.ID))
	if err := response.JSON(w, http.StatusOK, account); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateAccountPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	err := app.services.Identity.UpdatePassword(r.Context(), principal, identity.UpdatePasswordInput{
		CurrentPassword: input.CurrentPassword, NewPassword: input.NewPassword,
	})
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	app.logInfo(r, "account password changed", slog.Int64("account_id", principal.Subject.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getAccountOrgs(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"slug":       "slug",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}
	orgs, err := app.services.Identity.Orgs(r.Context(), principal, identity.ListOrgsInput{
		Search: q.Search, Sort: q.Sort, Order: q.Order, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, orgs); err != nil {
		app.serverError(w, r, err)
	}
}

type sessionResponse struct {
	Account         identity.Account        `json:"account"`
	Organizations   []identity.Organization `json:"organizations"`
	IsInstanceAdmin bool                    `json:"is_instance_admin"`
	FeatureFlags    []string                `json:"feature_flags"`
}

// getSession returns the authenticated account plus UI bootstrap metadata.
func (app *application) getSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := app.requestPrincipal(w, r)
	if !ok {
		return
	}
	current, err := app.services.Identity.CurrentSession(r.Context(), principal)
	if err != nil {
		app.identityError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, sessionResponse{
		Account:         current.Account,
		Organizations:   current.Organizations,
		IsInstanceAdmin: current.IsInstanceAdmin,
		FeatureFlags:    []string{},
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
