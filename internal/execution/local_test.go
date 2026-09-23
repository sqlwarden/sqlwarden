package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/connection"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
	"github.com/sqlwarden/internal/execution"
	"github.com/sqlwarden/internal/execution/executiontest"
)

func TestCredentialsCannotBeSerialized(t *testing.T) {
	values := []any{
		execution.Credentials{DSN: "plaintext-secret"},
		execution.SSHConfig{Password: "plaintext-secret"},
	}
	for _, value := range values {
		_, err := json.Marshal(value)
		if !errors.Is(err, execution.ErrCredentialSerialization) {
			t.Fatalf("Marshal(%T) error = %v, want credential serialization failure", value, err)
		}
	}
}

func TestCredentialsRedactFormatting(t *testing.T) {
	values := []any{
		execution.Credentials{DSN: "plaintext-secret", SSH: &execution.SSHConfig{Password: "plaintext-secret"}},
		execution.SSHConfig{Password: "plaintext-secret"},
	}
	for _, value := range values {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
			if formatted := fmt.Sprintf(verb, value); strings.Contains(formatted, "plaintext-secret") {
				t.Fatalf("Sprintf(%q, %T) leaked credentials: %s", verb, value, formatted)
			}
		}
	}
}

func TestCredentialsRedactStructuredLogging(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logger.Info("resolved", "credentials", execution.Credentials{
		DSN: "plaintext-secret", SSH: &execution.SSHConfig{Password: "plaintext-secret"},
	})
	if strings.Contains(output.String(), "plaintext-secret") {
		t.Fatalf("structured log leaked credentials: %s", output.String())
	}
}

func TestOpenRequestContainsNoCredentialMaterial(t *testing.T) {
	payload, err := json.Marshal(execution.OpenRequest{
		Scope:  execution.Scope{ConnectionID: "42"},
		Limits: execution.Limits{MaxRows: 100, MaxBytes: 1 << 20},
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"dsn", "tls", "ssh", "password", "private_key"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("OpenRequest JSON contains credential field %q: %s", forbidden, encoded)
		}
	}
}

type staticCredentialProvider map[string]execution.Credentials

func (p staticCredentialProvider) Resolve(_ context.Context, connectionID string) (execution.Credentials, error) {
	credentials, ok := p[connectionID]
	if !ok {
		return execution.Credentials{}, execution.ErrCredentialsNotFound
	}
	return credentials, nil
}

type targetValidatorFunc func(context.Context, string, string) error

func (fn targetValidatorFunc) Validate(ctx context.Context, driver, dsn string) error {
	return fn(ctx, driver, dsn)
}

func sqliteCredentials(t testing.TB, connectionIDs ...string) staticCredentialProvider {
	t.Helper()
	provider := make(staticCredentialProvider, len(connectionIDs))
	for _, connectionID := range connectionIDs {
		provider[connectionID] = execution.Credentials{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), connectionID+".db")}
	}
	return provider
}

func TestLocalRuntimeContract(t *testing.T) {
	executiontest.RunRuntimeContract(t, func(t testing.TB) (execution.SessionRuntime, execution.SessionDirectory) {
		sessions := connection.New(time.Minute)
		cursors := connection.NewQueryCursorManager(time.Minute)
		t.Cleanup(func() {
			cursors.Close()
			sessions.Close()
		})
		directory := execution.NewMemorySessionDirectory()
		return execution.NewLocalRuntime(sessions, cursors, directory, sqliteCredentials(t, "44"), nil, time.Minute), directory
	})
}

func TestLocalRuntimeEphemeralSessionDoesNotReplacePooledSession(t *testing.T) {
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	t.Cleanup(func() {
		cursors.Close()
		sessions.Close()
	})
	runtime := execution.NewLocalRuntime(sessions, cursors, execution.NewMemorySessionDirectory(), sqliteCredentials(t, "connection"), nil, time.Minute)
	ctx := context.Background()
	scope := execution.Scope{AccountID: "account", ConnectionID: "connection"}

	pooled, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	ephemeral, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope, Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	if ephemeral.Handle == pooled.Handle || ephemeral.Reused {
		t.Fatalf("ephemeral Open() = %+v, pooled = %+v", ephemeral, pooled)
	}
	if err := runtime.Close(ctx, execution.CloseRequest{Handle: ephemeral.Handle}); err != nil {
		t.Fatal(err)
	}
	reused, err := runtime.Open(ctx, execution.OpenRequest{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if reused.Handle != pooled.Handle || !reused.Reused {
		t.Fatalf("pooled session after ephemeral close = %+v, want reused %q", reused, pooled.Handle)
	}
}

func TestLocalRuntimeDoesNotClassifyValidatorInfrastructureFailureAsPolicyRejection(t *testing.T) {
	sessions := connection.New(time.Minute)
	cursors := connection.NewQueryCursorManager(time.Minute)
	t.Cleanup(func() {
		cursors.Close()
		sessions.Close()
	})
	settingsErr := errors.New("settings unavailable")
	runtime := execution.NewLocalRuntime(
		sessions, cursors, execution.NewMemorySessionDirectory(),
		staticCredentialProvider{"42": {Driver: "sqlite", DSN: ":memory:"}},
		targetValidatorFunc(func(context.Context, string, string) error { return settingsErr }),
		time.Minute,
	)

	_, err := runtime.Open(t.Context(), execution.OpenRequest{Scope: execution.Scope{ConnectionID: "42"}})
	if !errors.Is(err, settingsErr) {
		t.Fatalf("Open() error = %v, want settings failure", err)
	}
	if errors.Is(err, execution.ErrTargetRejected) {
		t.Fatalf("Open() classified settings failure as target rejection: %v", err)
	}
}
