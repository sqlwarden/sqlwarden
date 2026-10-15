package catalog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/credentials"
)

type fakeRevealPolicy struct {
	allowed bool
	err     error
}

func (p fakeRevealPolicy) Allowed(context.Context, credentials.OrgRef, access.Principal) (bool, error) {
	return p.allowed, p.err
}

type auditRecorder struct{ events []audit.Event }

func (r *auditRecorder) Write(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func revealFixture() (*Service, *fakeCredentials, *auditRecorder, *bytes.Buffer, access.Principal, ConnRef) {
	service, _, provider, principal := catalogFixture()
	recorder := &auditRecorder{}
	var logs bytes.Buffer
	service.deps.Audit = recorder
	service.deps.Logger = slog.New(slog.NewTextHandler(&logs, nil))
	ref := ConnRef{OrgID: 1, WorkspaceID: 10, ConnectionID: 30}
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceStored}
	provider.values[credentials.SecretPassword] = "highly-secret-value"
	return service, provider, recorder, &logs, principal, ref
}

func TestRevealSecretDeniesReferenceBeforePolicyAndPermission(t *testing.T) {
	service, provider, recorder, _, principal, ref := revealFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceReference}
	service.deps.RevealPolicy = fakeRevealPolicy{err: errors.New("policy must not run")}
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnRead: true}}

	_, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword)
	var revealErr *ErrReveal
	if !errors.As(err, &revealErr) || revealErr.Code != RevealCodeNotRevealable {
		t.Fatalf("RevealSecret error = %v, want %s", err, RevealCodeNotRevealable)
	}
	if provider.revealCalls != 0 || len(recorder.events) != 0 {
		t.Fatalf("denied reveal had side effects: provider=%d audit=%d", provider.revealCalls, len(recorder.events))
	}
}

func TestRevealSecretDeniesDisabledPolicyBeforePermission(t *testing.T) {
	service, provider, recorder, _, principal, ref := revealFixture()
	service.deps.RevealPolicy = fakeRevealPolicy{allowed: false}
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnRead: true}}

	_, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword)
	var revealErr *ErrReveal
	if !errors.As(err, &revealErr) || revealErr.Code != RevealCodeDisabled {
		t.Fatalf("RevealSecret error = %v, want %s", err, RevealCodeDisabled)
	}
	if provider.revealCalls != 0 || len(recorder.events) != 0 {
		t.Fatalf("denied reveal had side effects: provider=%d audit=%d", provider.revealCalls, len(recorder.events))
	}
}

func TestRevealSecretDeniesMissingPermission(t *testing.T) {
	service, provider, recorder, _, principal, ref := revealFixture()
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnRead: true}}

	_, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("RevealSecret error = %v, want ErrForbidden", err)
	}
	if provider.revealCalls != 0 || len(recorder.events) != 0 {
		t.Fatalf("denied reveal had side effects: provider=%d audit=%d", provider.revealCalls, len(recorder.events))
	}
}

func TestRevealSecretSuccessAuditsAndLogsWithoutValue(t *testing.T) {
	service, provider, recorder, logs, principal, ref := revealFixture()
	value, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword)
	if err != nil || value != provider.values[credentials.SecretPassword] {
		t.Fatalf("RevealSecret = %q, %v", value, err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("audit events = %d, want 1", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Action != "connection.secret_revealed" || event.ResourceID != "30" || event.Metadata["secret_name"] != "password" || event.Actor.SubjectID == nil || *event.Actor.SubjectID != principal.Subject.ID {
		t.Fatalf("audit event = %+v", event)
	}
	serialized := event.Action + event.ResourceID + event.Metadata["secret_name"] + logs.String()
	if strings.Contains(serialized, value) {
		t.Fatal("secret value appeared in audit event or log")
	}
}

func TestRevealableMatchesRevealGatesWithoutOpeningValue(t *testing.T) {
	service, provider, _, _, principal, ref := revealFixture()
	allowed, err := service.Revealable(context.Background(), principal, ref, credentials.SecretPassword)
	if err != nil || !allowed {
		t.Fatalf("Revealable = %v, %v", allowed, err)
	}
	if provider.revealCalls != 0 {
		t.Fatalf("Revealable opened the secret %d times", provider.revealCalls)
	}
	service.deps.RevealPolicy = fakeRevealPolicy{allowed: false}
	allowed, err = service.Revealable(context.Background(), principal, ref, credentials.SecretPassword)
	if err != nil || allowed {
		t.Fatalf("Revealable with disabled policy = %v, %v", allowed, err)
	}
}

type failingAudit struct{}

func (failingAudit) Write(context.Context, audit.Event) error { return errors.New("audit down") }

func TestRevealSecretRefusesWithoutAuditWriter(t *testing.T) {
	service, provider, _, _, principal, ref := revealFixture()
	service.deps.Audit = nil
	value, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword)
	if err == nil || value != "" || provider.revealCalls != 0 {
		t.Fatalf("RevealSecret = %q, %v reveals=%d", value, err, provider.revealCalls)
	}
}

func TestRevealSecretDoesNotReturnValueWhenAuditFails(t *testing.T) {
	service, _, _, logs, principal, ref := revealFixture()
	service.deps.Audit = failingAudit{}
	value, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword)
	if err == nil || value != "" {
		t.Fatalf("RevealSecret = %q, %v", value, err)
	}
	if strings.Contains(logs.String(), "highly-secret-value") {
		t.Fatal("secret value logged")
	}
}

func TestRevealSecretForeignOrUnknownConnectionIsNotFound(t *testing.T) {
	service, _, recorder, _, principal, _ := revealFixture()
	for _, ref := range []ConnRef{
		{OrgID: 1, WorkspaceID: 10, ConnectionID: 999},
		{OrgID: 1, WorkspaceID: 999, ConnectionID: 30},
	} {
		if _, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword); !errors.Is(err, ErrNotFound) {
			t.Fatalf("RevealSecret(%+v) = %v", ref, err)
		}
	}
	if len(recorder.events) != 0 {
		t.Fatal("audit event written for not-found connection")
	}
}

func TestRevealSecretRejectsUnknownSecretName(t *testing.T) {
	service, provider, recorder, _, principal, ref := revealFixture()
	_, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretName("bogus"))
	if !errors.Is(err, ErrValidation) || provider.revealCalls != 0 || len(recorder.events) != 0 {
		t.Fatalf("RevealSecret error = %v", err)
	}
}

func TestRevealableFalseForMissingPermissionAndReferenceSource(t *testing.T) {
	service, provider, _, _, principal, ref := revealFixture()
	service.deps.Policy = fakePolicy{permissions: map[string]bool{access.PermConnRead: true}}
	allowed, err := service.Revealable(context.Background(), principal, ref, credentials.SecretPassword)
	if err != nil || allowed {
		t.Fatalf("missing permission: Revealable = %v, %v", allowed, err)
	}
	service, provider, _, _, principal, ref = revealFixture()
	provider.states[credentials.SecretPassword] = credentials.SecretState{Set: true, Source: credentials.SourceReference}
	allowed, err = service.Revealable(context.Background(), principal, ref, credentials.SecretPassword)
	if err != nil || allowed {
		t.Fatalf("reference source: Revealable = %v, %v", allowed, err)
	}
}

func TestRevealSecretRejectsNonAccountSubject(t *testing.T) {
	service, provider, recorder, _, principal, ref := revealFixture()
	principal.Subject.Kind = access.SubjectKind("service")
	if _, err := service.RevealSecret(context.Background(), principal, ref, credentials.SecretPassword); !errors.Is(err, ErrForbidden) {
		t.Fatalf("RevealSecret error = %v, want ErrForbidden", err)
	}
	if provider.revealCalls != 0 || len(recorder.events) != 0 {
		t.Fatal("denied reveal had side effects")
	}
}
