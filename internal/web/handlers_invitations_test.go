package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/smtp"
	"github.com/sqlwarden/internal/token"
)

func TestOrganizationInvitationExistingAccountLifecycle(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-owner"), "Invite Owner", "securepass99")
	invitee, inviteeToken := seedAccountWithToken(t, app, uniqueEmail(t, "invite-existing"), "Existing Invitee")

	created := createInvitationForTest(t, app, slug, invitee.Email, ownerToken)
	assert.Equal(t, created.StatusCode, http.StatusCreated)
	assert.Equal(t, created.BodyFields["delivery_status"], database.InvitationDeliverySent)
	if len(app.mailer.SentMessages) != 1 || !strings.Contains(app.mailer.SentMessages[0], invitee.Email) {
		t.Fatal("expected invitation email to be sent to the invitee")
	}

	plain := invitationTokenFromResponse(t, created)
	var stored database.OrganizationInvitation
	if err := app.db.NewSelect().Model(&stored).Where("normalized_email = ?", database.NormalizeInvitationEmail(invitee.Email)).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, stored.TokenHash, token.Hash(plain))
	assert.NotEqual(t, stored.TokenHash, plain)

	resolved := send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+plain, nil), app.routes())
	assert.Equal(t, resolved.StatusCode, http.StatusOK)
	assert.Equal(t, resolved.BodyFields["account_exists"], true)

	unauthenticated := send(t, newTestRequest(t, http.MethodPost, "/api/v1/invitations/"+plain+"/accept", map[string]any{}), app.routes())
	assert.Equal(t, unauthenticated.StatusCode, http.StatusUnauthorized)

	accepted := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/invitations/"+plain+"/accept", map[string]any{}, inviteeToken), app.routes())
	assert.Equal(t, accepted.StatusCode, http.StatusOK)
	org, found, err := app.db.GetOrgBySlug(context.Background(), slug)
	if err != nil || !found {
		t.Fatalf("get org: found=%v err=%v", found, err)
	}
	member, err := app.db.IsOrgMember(context.Background(), org.ID, invitee.ID)
	if err != nil || !member {
		t.Fatalf("accepted invitee should be a member: member=%v err=%v", member, err)
	}
	assert.Equal(t, send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+plain, nil), app.routes()).StatusCode, http.StatusNotFound)
}

func TestOrganizationInvitationCreatesAccountAndSession(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-new-owner"), "Invite Owner", "securepass99")
	email := uniqueEmail(t, "invite-new")
	created := createInvitationForTest(t, app, slug, email, ownerToken)
	plain := invitationTokenFromResponse(t, created)

	accepted := send(t, newTestRequest(t, http.MethodPost, "/api/v1/invitations/"+plain+"/accept", map[string]any{
		"name": "New Invitee", "password": "securepass99",
	}), app.routes())
	assert.Equal(t, accepted.StatusCode, http.StatusCreated)
	accessToken, ok := accepted.BodyFields["access_token"].(string)
	if !ok || accessToken == "" {
		t.Fatal("expected new-account acceptance to issue an access token")
	}
	account, found, err := app.db.GetAccountByEmail(context.Background(), email)
	if err != nil || !found {
		t.Fatalf("get accepted account: found=%v err=%v", found, err)
	}
	org, _, _ := app.db.GetOrgBySlug(context.Background(), slug)
	member, err := app.db.IsOrgMember(context.Background(), org.ID, account.ID)
	if err != nil || !member {
		t.Fatalf("new account should be an org member: member=%v err=%v", member, err)
	}
	session := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/session", nil, accessToken), app.routes())
	assert.Equal(t, session.StatusCode, http.StatusOK)
}

func TestOrganizationInvitationListResendAndRevoke(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-manage-owner"), "Invite Owner", "securepass99")
	created := createInvitationForTest(t, app, slug, uniqueEmail(t, "invite-manage"), ownerToken)
	firstToken := invitationTokenFromResponse(t, created)
	invitation := created.BodyFields["invitation"].(map[string]any)
	id := invitation["id"].(string)

	listed := send(t, newAuthRequest(t, http.MethodGet, "/api/v1/orgs/"+slug+"/invitations", nil, ownerToken), app.routes())
	assert.Equal(t, listed.StatusCode, http.StatusOK)
	assert.Equal(t, int(listed.BodyFields["total"].(float64)), 1)

	resent := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/invitations/"+id+"/resend", nil, ownerToken), app.routes())
	assert.Equal(t, resent.StatusCode, http.StatusOK)
	secondToken := invitationTokenFromResponse(t, resent)
	assert.NotEqual(t, firstToken, secondToken)
	assert.Equal(t, send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+firstToken, nil), app.routes()).StatusCode, http.StatusNotFound)
	assert.Equal(t, send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+secondToken, nil), app.routes()).StatusCode, http.StatusOK)

	revoked := send(t, newAuthRequest(t, http.MethodDelete, "/api/v1/orgs/"+slug+"/invitations/"+id, nil, ownerToken), app.routes())
	assert.Equal(t, revoked.StatusCode, http.StatusNoContent)
	assert.Equal(t, send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+secondToken, nil), app.routes()).StatusCode, http.StatusNotFound)
}

func TestOrganizationInvitationDeliveryDisabledAndExpiration(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.mailer = smtp.NewDisabledMailer("")
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-disabled-owner"), "Invite Owner", "securepass99")
	created := createInvitationForTest(t, app, slug, uniqueEmail(t, "invite-disabled"), ownerToken)
	assert.Equal(t, created.BodyFields["delivery_status"], database.InvitationDeliveryDisabled)
	if created.BodyFields["invite_url"] == "" {
		t.Fatal("expected a copyable invitation URL when SMTP is disabled")
	}
	plain := invitationTokenFromResponse(t, created)
	if _, err := app.db.NewUpdate().Model((*database.OrganizationInvitation)(nil)).Set("expires_at = ?", time.Now().Add(-time.Minute)).Where("token_hash = ?", token.Hash(plain)).Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
	resolved := send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+plain, nil), app.routes())
	assert.Equal(t, resolved.StatusCode, http.StatusOK)
	assert.Equal(t, resolved.BodyFields["status"], "expired")
	accepted := send(t, newTestRequest(t, http.MethodPost, "/api/v1/invitations/"+plain+"/accept", map[string]any{"name": "Expired", "password": "securepass99"}), app.routes())
	assert.Equal(t, accepted.StatusCode, http.StatusGone)
}

func TestOrganizationInvitationDeliveryFailureKeepsCopyableLink(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	app.mailer = smtp.NewMockMailer("")
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-failed-owner"), "Invite Owner", "securepass99")
	created := createInvitationForTest(t, app, slug, uniqueEmail(t, "invite-failed"), ownerToken)
	assert.Equal(t, created.StatusCode, http.StatusCreated)
	assert.Equal(t, created.BodyFields["delivery_status"], database.InvitationDeliveryFailed)
	plain := invitationTokenFromResponse(t, created)
	assert.Equal(t, send(t, newTestRequest(t, http.MethodGet, "/api/v1/invitations/"+plain, nil), app.routes()).StatusCode, http.StatusOK)
}

func TestOrganizationInvitationsUnavailableInSingleUserMode(t *testing.T) {
	t.Parallel()
	app := newTestApp(t)
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-single-owner"), "Invite Owner", "securepass99")
	app.config.Mode = ModeDesktop
	res := createInvitationForTest(t, app, slug, uniqueEmail(t, "invite-single"), ownerToken)
	assert.Equal(t, res.StatusCode, http.StatusNotFound)
}

func createInvitationForTest(t *testing.T, app *application, slug, email, authToken string) testResponse {
	t.Helper()
	return send(t, newAuthRequest(t, http.MethodPost, "/api/v1/orgs/"+slug+"/invitations", map[string]any{"email": email}, authToken), app.routes())
}

func TestInvitationURLUsesRuntimeBaseURL(t *testing.T) {
	app := newTestApp(t)
	_, ownerToken, slug := registerAndLogin(t, app, uniqueEmail(t, "invite-base-url-owner"), "Invite Owner", "securepass99")
	updateInstanceSettingsForTest(t, app, func(settings *database.InstanceSettings) {
		settings.BaseURL = "https://runtime.example.com/sqlwarden"
	})

	res := createInvitationForTest(t, app, slug, uniqueEmail(t, "invite-base-url"), ownerToken)
	assert.Equal(t, res.StatusCode, http.StatusCreated)
	inviteURL, ok := res.BodyFields["invite_url"].(string)
	if !ok || !strings.HasPrefix(inviteURL, "https://runtime.example.com/sqlwarden/invitations/") {
		t.Fatalf("invite URL = %q", inviteURL)
	}
}

func invitationTokenFromResponse(t *testing.T, res testResponse) string {
	t.Helper()
	inviteURL, ok := res.BodyFields["invite_url"].(string)
	if !ok || inviteURL == "" {
		t.Fatalf("response did not contain an invitation URL: %s", res.BodyBytes)
	}
	return inviteURL[strings.LastIndex(inviteURL, "/")+1:]
}
