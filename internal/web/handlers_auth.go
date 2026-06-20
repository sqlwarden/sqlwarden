package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
)

func (app *application) registerAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string              `json:"email"`
		Name     string              `json:"name"`
		Password string              `json:"password"`
		V        validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Email != "", "email", "Email is required.")
	input.V.CheckField(validator.IsEmail(input.Email), "email", "Enter a valid email address.")
	input.V.CheckField(input.Name != "", "name", "Name is required.")
	input.V.CheckField(len(input.Password) >= 8, "password", "Password must be at least 8 characters.")

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	configured, err := app.db.HasAnyInstanceAdmin(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !configured {
		app.errorMessage(w, r, http.StatusForbidden, "Instance setup is not complete.", nil)
		return
	}

	_, exists, err := app.db.GetAccountByEmail(r.Context(), input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if exists {
		input.V.AddFieldError("email", "An account with this email already exists.")
		app.failedValidation(w, r, input.V)
		return
	}

	hashedPW, err := password.Hash(input.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	account, err := app.db.InsertAccount(r.Context(), input.Email, input.Name, &hashedPW)
	if err != nil {
		if isUniqueViolation(err) {
			input.V.AddFieldError("email", "An account with this email already exists.")
			app.failedValidation(w, r, input.V)
			return
		}
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "account registered", slog.Int64("account_id", account.ID))
	err = response.JSON(w, http.StatusCreated, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) loginAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string              `json:"email"`
		Password string              `json:"password"`
		V        validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.Email != "", "email", "Email is required.")
	input.V.CheckField(input.Password != "", "password", "Password is required.")

	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account, found, err := app.db.GetAccountByEmail(r.Context(), input.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	if !found || account.Password == nil || !account.IsActive {
		app.invalidAuthenticationToken(w, r)
		return
	}

	match, err := password.Matches(input.Password, *account.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !match {
		app.invalidAuthenticationToken(w, r)
		return
	}

	accountIDStr := strconv.FormatInt(account.ID, 10)
	sessionExpiresAt := time.Now().Add(7 * 24 * time.Hour)
	family := database.NewID()
	authSession, _, err := app.db.CreateAuthSessionWithRefreshToken(
		r.Context(),
		account.ID,
		sessionExpiresAt,
		r.Header.Get("User-Agent"),
		r.RemoteAddr,
		token.Hash(family),
		family,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	accessToken, _, err := token.IssueWithSessionTTL(accountIDStr, authSession.ID, account.Email, account.Name, app.config.JWT.SecretKey, app.config.JWT.AccessTokenTTL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "account logged in", slog.Int64("account_id", account.ID), slog.String("auth_session_id", authSession.ID))
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    family,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   7 * 24 * 3600,
	})

	err = response.JSON(w, http.StatusOK, map[string]string{"access_token": accessToken})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) refreshToken(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		app.invalidAuthenticationToken(w, r)
		return
	}

	rt, found, err := app.db.GetRefreshTokenByHash(r.Context(), token.Hash(cookie.Value))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		if found && rt.RevokedAt == nil {
			_ = app.db.RevokeFamilyTokens(r.Context(), rt.Family)
		}
		if found {
			reason := "refresh_token_revoked"
			if rt.RevokedAt == nil && time.Now().After(rt.ExpiresAt) {
				reason = "refresh_token_expired"
			}
			app.logWarn(r, "refresh token rejected", slog.Int64("account_id", rt.AccountID), slog.String("auth_session_id", rt.AuthSessionID), slog.String("reason", reason))
		}
		app.invalidAuthenticationToken(w, r)
		return
	}
	if rt.AuthSessionID == "" {
		app.invalidAuthenticationToken(w, r)
		return
	}

	account, found, err := app.db.GetAccount(r.Context(), rt.AccountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.invalidAuthenticationToken(w, r)
		return
	}
	if !account.IsActive {
		app.invalidAuthenticationToken(w, r)
		return
	}

	authSession, found, err := app.db.GetAuthSession(r.Context(), rt.AuthSessionID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found || authSession.RevokedAt != nil || time.Now().After(authSession.ExpiresAt) {
		reason := "auth_session_not_found"
		if found && authSession.RevokedAt != nil {
			reason = "auth_session_revoked"
		} else if found && time.Now().After(authSession.ExpiresAt) {
			reason = "auth_session_expired"
		}
		app.logWarn(r, "refresh auth session rejected", slog.Int64("account_id", account.ID), slog.String("auth_session_id", rt.AuthSessionID), slog.String("reason", reason))
		app.invalidAuthenticationToken(w, r)
		return
	}

	accountIDStr := strconv.FormatInt(account.ID, 10)
	accessToken, _, err := token.IssueWithSessionTTL(accountIDStr, authSession.ID, account.Email, account.Name, app.config.JWT.SecretKey, app.config.JWT.AccessTokenTTL)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	family := database.NewID()
	_, err = app.db.RotateRefreshToken(r.Context(),
		rt.ID,
		rt.AccountID,
		authSession.ID,
		token.Hash(family),
		rt.Family,
		time.Now().Add(7*24*time.Hour),
		r.Header.Get("User-Agent"),
		r.RemoteAddr,
	)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "access token refreshed", slog.Int64("account_id", account.ID), slog.String("auth_session_id", authSession.ID))
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    family,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   7 * 24 * 3600,
	})

	err = response.JSON(w, http.StatusOK, map[string]string{"access_token": accessToken})
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) logoutAccount(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err == nil {
		rt, found, err := app.db.GetRefreshTokenByHash(r.Context(), token.Hash(cookie.Value))
		if err == nil && found {
			_ = app.db.RevokeRefreshToken(r.Context(), rt.ID)
			if rt.AuthSessionID != "" {
				_ = app.db.RevokeAuthSession(r.Context(), rt.AuthSessionID, &rt.AccountID, "logout")
				app.logInfo(r, "account logged out", slog.Int64("account_id", rt.AccountID), slog.String("auth_session_id", rt.AuthSessionID))
			}
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth",
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listAccountSessions(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	sessions, err := app.db.ListAuthSessionsPage(r.Context(), database.ListAuthSessionsParams{
		AccountID: account.ID,
		Page:      q.Page,
		PageSize:  q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, sessions)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeAccountSession(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	if sessionID == "" {
		app.notFound(w, r)
		return
	}

	session, found, err := app.db.GetAuthSession(r.Context(), sessionID, account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if err = app.db.RevokeAuthSession(r.Context(), session.ID, &account.ID, "user_revoked"); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "auth session revoked", slog.Int64("target_account_id", account.ID), slog.String("auth_session_id", session.ID), slog.String("reason", "user_revoked"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeAccountSessions(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	if err := app.db.RevokeAuthSessionsForAccount(r.Context(), account.ID, &account.ID, "user_revoked_all"); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "auth sessions revoked", slog.Int64("target_account_id", account.ID), slog.String("reason", "user_revoked_all"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listInstanceAccountSessions(w http.ResponseWriter, r *http.Request) {
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	if _, found, err := app.db.GetAccount(r.Context(), accountID); err != nil {
		app.serverError(w, r, err)
		return
	} else if !found {
		app.notFound(w, r)
		return
	}

	sessions, err := app.db.ListAuthSessionsPage(r.Context(), database.ListAuthSessionsParams{
		AccountID: accountID,
		Page:      q.Page,
		PageSize:  q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, sessions)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeInstanceAccountSession(w http.ResponseWriter, r *http.Request) {
	admin := contextGetAccount(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	session, found, err := app.db.GetAuthSession(r.Context(), sessionID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if err = app.db.RevokeAuthSession(r.Context(), session.ID, &admin.ID, "instance_admin_revoked"); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "auth session revoked", slog.Int64("target_account_id", accountID), slog.String("auth_session_id", session.ID), slog.String("reason", "instance_admin_revoked"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) revokeInstanceAccountSessions(w http.ResponseWriter, r *http.Request) {
	admin := contextGetAccount(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	if _, found, err := app.db.GetAccount(r.Context(), accountID); err != nil {
		app.serverError(w, r, err)
		return
	} else if !found {
		app.notFound(w, r)
		return
	}
	if err = app.db.RevokeAuthSessionsForAccount(r.Context(), accountID, &admin.ID, "instance_admin_revoked_all"); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "auth sessions revoked", slog.Int64("target_account_id", accountID), slog.String("reason", "instance_admin_revoked_all"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) listOrgMemberAccessSessions(w http.ResponseWriter, r *http.Request) {
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	if member, err := app.db.IsOrgMember(r.Context(), org.ID, accountID); err != nil {
		app.serverError(w, r, err)
		return
	} else if !member {
		app.notFound(w, r)
		return
	}

	sessions, err := app.db.ListOrgAccessSessionsPage(r.Context(), org.ID, accountID, q.Page, q.PageSize)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, sessions)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeOrgMemberAccessSession(w http.ResponseWriter, r *http.Request) {
	admin := contextGetAccount(r)
	org := contextGetOrg(r)
	accountID, err := strconv.ParseInt(chi.URLParam(r, "account_id"), 10, 64)
	if err != nil {
		app.notFound(w, r)
		return
	}
	sessionID := strings.TrimSpace(chi.URLParam(r, "session_id"))
	session, found, err := app.db.GetOrgAccessSessionByID(r.Context(), sessionID, org.ID, accountID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if err = app.db.RevokeOrgAccessSession(r.Context(), session.ID, org.ID, &admin.ID, "org_admin_revoked"); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.logInfo(r, "org access session revoked", slog.Int64("target_account_id", accountID), slog.String("org_access_session_id", session.ID), slog.String("reason", "org_admin_revoked"))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getAccount(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	err := response.JSON(w, http.StatusOK, account)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateAccount(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string              `json:"name"`
		V    validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.V.CheckField(input.Name != "", "name", "Name is required.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account := contextGetAccount(r)
	updatedAccount, err := app.db.UpdateAccountName(r.Context(), account.ID, input.Name)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "account updated", slog.Int64("account_id", account.ID))
	err = response.JSON(w, http.StatusOK, updatedAccount)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) updateAccountPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string              `json:"current_password"`
		NewPassword     string              `json:"new_password"`
		V               validator.Validator `json:"-"`
	}

	err := request.DecodeJSON(w, r, &input)
	if err != nil {
		app.badRequest(w, r, err)
		return
	}

	input.V.CheckField(input.CurrentPassword != "", "current_password", "Current password is required.")
	input.V.CheckField(len(input.NewPassword) >= 8, "new_password", "New password must be at least 8 characters.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	account := contextGetAccount(r)
	if account.Password == nil {
		input.V.AddFieldError("current_password", "Password changes are not available for this account.")
		app.failedValidation(w, r, input.V)
		return
	}

	match, err := password.Matches(input.CurrentPassword, *account.Password)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !match {
		input.V.AddFieldError("current_password", "Current password is incorrect.")
		app.failedValidation(w, r, input.V)
		return
	}

	hashedPassword, err := password.Hash(input.NewPassword)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = app.db.UpdateAccountPassword(r.Context(), account.ID, hashedPassword)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	app.logInfo(r, "account password changed", slog.Int64("account_id", account.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getAccountOrgs(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)
	q, errs := readListQuery(r.URL.Query(), map[string]string{
		"name":       "name",
		"slug":       "slug",
		"created_at": "created_at",
	})
	if len(errs) != 0 {
		app.failedValidation(w, r, fieldErrors(errs))
		return
	}

	orgs, err := app.db.ListAccountOrgsPage(r.Context(), database.ListAccountOrgsParams{
		AccountID: account.ID,
		Search:    q.Search,
		Sort:      q.Sort,
		Order:     q.Order,
		Page:      q.Page,
		PageSize:  q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	err = response.JSON(w, http.StatusOK, orgs)
	if err != nil {
		app.serverError(w, r, err)
	}
}

type sessionResponse struct {
	Account               database.Account        `json:"account"`
	Organizations         []database.Organization `json:"organizations"`
	IsInstanceAdmin       bool                    `json:"is_instance_admin"`
	PersonalSpacesEnabled bool                    `json:"personal_spaces_enabled"`
	FeatureFlags          []string                `json:"feature_flags"`
}

// getSession returns the authenticated account plus UI bootstrap metadata.
func (app *application) getSession(w http.ResponseWriter, r *http.Request) {
	account := contextGetAccount(r)

	orgs, err := app.db.GetAccountOrgs(r.Context(), account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	isAdmin, err := app.db.IsInstanceAdmin(r.Context(), account.ID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if orgs == nil {
		orgs = []database.Organization{}
	}

	personalSpacesEnabled, err := app.personalSpacesEnabled(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	err = response.JSON(w, http.StatusOK, sessionResponse{
		Account:               account,
		Organizations:         orgs,
		IsInstanceAdmin:       isAdmin,
		PersonalSpacesEnabled: personalSpacesEnabled,
		FeatureFlags:          []string{},
	})
	if err != nil {
		app.serverError(w, r, err)
	}
}
