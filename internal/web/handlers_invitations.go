package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/password"
	"github.com/sqlwarden/internal/request"
	"github.com/sqlwarden/internal/response"
	"github.com/sqlwarden/internal/smtp"
	"github.com/sqlwarden/internal/token"
	"github.com/sqlwarden/internal/validator"
	"github.com/uptrace/bun"
)

const organizationInvitationTTL = 7 * 24 * time.Hour

var errOrganizationInvitationUnavailable = errors.New("organization invitation is no longer available")

type invitationResponse struct {
	Invitation     database.OrganizationInvitation `json:"invitation"`
	InviteURL      string                          `json:"invite_url,omitempty"`
	DeliveryStatus string                          `json:"delivery_status"`
}

func (app *application) listOrganizationInvitations(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	q, fieldErrors := readListQuery(r.URL.Query(), map[string]string{"created_at": "created_at"})
	if len(fieldErrors) > 0 {
		v := validator.Validator{FieldErrors: fieldErrors}
		app.failedValidation(w, r, v)
		return
	}
	org := contextGetOrg(r)
	page, err := app.db.ListOrganizationInvitationsPage(r.Context(), database.ListOrganizationInvitationsParams{
		OrgID: org.ID, Search: q.Search, Page: q.Page, PageSize: q.PageSize,
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, page); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) createOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	var input struct {
		Email string              `json:"email"`
		V     validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}
	input.Email = strings.TrimSpace(input.Email)
	input.V.CheckField(input.Email != "", "email", "Email is required.")
	input.V.CheckField(validator.IsEmail(input.Email), "email", "Enter a valid email address.")
	if input.V.HasErrors() {
		app.failedValidation(w, r, input.V)
		return
	}

	org := contextGetOrg(r)
	if account, found, err := app.db.GetAccountByEmail(r.Context(), input.Email); err != nil {
		app.serverError(w, r, err)
		return
	} else if found {
		member, err := app.db.IsOrgMember(r.Context(), org.ID, account.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		if member {
			app.failedDuplicateField(w, r, "email", "This account is already a member of the organization.")
			return
		}
	}

	plain, hash, err := token.Generate()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	inviteURL, err := app.organizationInvitationURL(r.Context(), plain)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	inviter := contextGetAccount(r)
	invitation, err := app.db.InsertOrganizationInvitation(r.Context(), org.ID, input.Email, hash, inviter.ID, time.Now().Add(organizationInvitationTTL))
	if err != nil {
		if isUniqueViolation(err) {
			app.failedDuplicateField(w, r, "email", "A pending invitation already exists for this email.")
			return
		}
		app.serverError(w, r, err)
		return
	}
	deliveryStatus := app.deliverOrganizationInvitation(r, invitation, org, inviter.Name, inviteURL)
	invitation.LastDeliveryStatus = deliveryStatus
	if err := response.JSON(w, http.StatusCreated, invitationResponse{Invitation: invitation, InviteURL: inviteURL, DeliveryStatus: deliveryStatus}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) resendOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	org := contextGetOrg(r)
	plain, hash, err := token.Generate()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	inviteURL, err := app.organizationInvitationURL(r.Context(), plain)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	invitation, found, err := app.db.RotateOrganizationInvitation(r.Context(), org.ID, chi.URLParam(r, "invitation_id"), hash, time.Now().Add(organizationInvitationTTL))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	inviter := contextGetAccount(r)
	deliveryStatus := app.deliverOrganizationInvitation(r, invitation, org, inviter.Name, inviteURL)
	invitation.LastDeliveryStatus = deliveryStatus
	if err := response.JSON(w, http.StatusOK, invitationResponse{Invitation: invitation, InviteURL: inviteURL, DeliveryStatus: deliveryStatus}); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) revokeOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	org := contextGetOrg(r)
	revoked, err := app.db.RevokeOrganizationInvitation(r.Context(), org.ID, chi.URLParam(r, "invitation_id"))
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !revoked {
		app.notFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (app *application) getOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	invitation, org, found, err := app.resolveOrganizationInvitation(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	status := "pending"
	if !time.Now().Before(invitation.ExpiresAt) {
		status = "expired"
	}
	account, accountExists, err := app.db.GetAccountByEmail(r.Context(), invitation.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	current := contextGetAccount(r)
	payload := map[string]any{
		"organization": org, "email": invitation.Email, "expires_at": invitation.ExpiresAt,
		"status": status, "account_exists": accountExists, "authenticated_as_invitee": accountExists && current.ID == account.ID,
	}
	if err := response.JSON(w, http.StatusOK, payload); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) acceptOrganizationInvitation(w http.ResponseWriter, r *http.Request) {
	if !app.organizationInvitationsAvailable(w, r) {
		return
	}
	invitation, org, found, err := app.resolveOrganizationInvitation(r)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if !found {
		app.notFound(w, r)
		return
	}
	if !time.Now().Before(invitation.ExpiresAt) {
		app.errorMessage(w, r, http.StatusGone, "This invitation has expired. Ask an organization administrator to resend it.", nil)
		return
	}
	var input struct {
		Name     string              `json:"name"`
		Password string              `json:"password"`
		V        validator.Validator `json:"-"`
	}
	if err := request.DecodeJSON(w, r, &input); err != nil {
		app.badRequest(w, r, err)
		return
	}

	account, exists, err := app.db.GetAccountByEmail(r.Context(), invitation.Email)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	created := false
	if exists {
		current := contextGetAccount(r)
		if current.ID == 0 {
			app.authenticationRequired(w, r)
			return
		}
		if current.ID != account.ID || !account.IsActive {
			app.notPermitted(w, r)
			return
		}
	} else {
		if contextGetAccount(r).ID != 0 {
			app.errorMessage(w, r, http.StatusConflict, "Sign out before creating the invited account.", nil)
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		input.V.CheckField(input.Name != "", "name", "Name is required.")
		input.V.CheckField(len(input.Password) >= 8, "password", "Password must be at least 8 characters.")
		if input.V.HasErrors() {
			app.failedValidation(w, r, input.V)
			return
		}
	}

	err = app.db.RunInTx(r.Context(), nil, func(ctx context.Context, tx bun.Tx) error {
		if !exists {
			hashed, err := password.Hash(input.Password)
			if err != nil {
				return err
			}
			account, err = app.db.InsertAccountWithExecutor(ctx, tx, invitation.Email, input.Name, &hashed)
			if err != nil {
				return err
			}
			created = true
		}
		if err := app.db.AddOrgMemberWithExecutor(ctx, tx, org.ID, account.ID); err != nil {
			return err
		}
		accepted, err := app.db.MarkOrganizationInvitationAcceptedWithExecutor(ctx, tx, invitation.ID, account.ID)
		if err != nil {
			return err
		}
		if !accepted {
			return errOrganizationInvitationUnavailable
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errOrganizationInvitationUnavailable) {
			app.errorMessage(w, r, http.StatusConflict, "This invitation is no longer available.", nil)
			return
		}
		if isUniqueViolation(err) {
			app.errorMessage(w, r, http.StatusConflict, "An account for this email now exists. Sign in and accept the invitation again.", nil)
			return
		}
		app.serverError(w, r, err)
		return
	}
	if app.enforcer != nil {
		app.enforcer.InvalidatePrincipals(org.ID, account.ID)
	}
	result := map[string]any{"organization": org}
	status := http.StatusOK
	if created {
		accessToken, sessionID, err := app.issueAccountSession(w, r, account)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		result["access_token"] = accessToken
		status = http.StatusCreated
		app.logInfo(r, "invited account created", slog.Int64("account_id", account.ID), slog.Int64("org_id", org.ID), slog.String("auth_session_id", sessionID))
	}
	if err := response.JSON(w, status, result); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) organizationInvitationsAvailable(w http.ResponseWriter, r *http.Request) bool {
	if !app.config.productCapabilities().Invitations {
		app.notFound(w, r)
		return false
	}
	return true
}

func (app *application) resolveOrganizationInvitation(r *http.Request) (database.OrganizationInvitation, database.Organization, bool, error) {
	invitation, found, err := app.db.GetOrganizationInvitationByTokenHash(r.Context(), token.Hash(chi.URLParam(r, "token")))
	if err != nil || !found || invitation.AcceptedAt != nil || invitation.RevokedAt != nil {
		return database.OrganizationInvitation{}, database.Organization{}, false, err
	}
	org, found, err := app.db.GetOrg(r.Context(), invitation.OrgID)
	return invitation, org, found, err
}

func (app *application) organizationInvitationURL(ctx context.Context, plainToken string) (string, error) {
	settings, err := app.instanceSettings(ctx)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(settings.BaseURL, "/") + "/invitations/" + plainToken, nil
}

func (app *application) deliverOrganizationInvitation(r *http.Request, invitation database.OrganizationInvitation, org database.Organization, inviterName, inviteURL string) string {
	status := database.InvitationDeliveryDisabled
	data := map[string]any{"OrganizationName": org.Name, "InviterName": inviterName, "InviteURL": inviteURL, "ExpiresAt": invitation.ExpiresAt}
	if err := app.sendEmail(true, invitation.Email, data, "organization-invitation.tmpl"); err != nil {
		if !errors.Is(err, smtp.ErrDisabled) {
			status = database.InvitationDeliveryFailed
			app.logger.WarnContext(r.Context(), "organization invitation email delivery failed", "invitation_id", invitation.ID, "error", err)
		}
	} else {
		status = database.InvitationDeliverySent
	}
	if err := app.db.UpdateOrganizationInvitationDelivery(r.Context(), invitation.ID, status); err != nil {
		app.logger.ErrorContext(r.Context(), "organization invitation delivery status update failed", "invitation_id", invitation.ID, "error", err)
	}
	return status
}
