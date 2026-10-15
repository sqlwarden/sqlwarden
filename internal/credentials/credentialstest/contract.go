// Package credentialstest holds the behavior every credentials.Provider must
// satisfy.
package credentialstest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/credentials"
)

// Fixture describes data seeded into the provider under test.
type Fixture struct {
	// Ref resolves successfully to a connection with DSN WantDSN.
	Ref     credentials.ConnectionRef
	WantDSN string
	// WithSSH resolves to credentials that carry an SSH tunnel config.
	WithSSH credentials.ConnectionRef
	// WrongWorkspace and WrongOrg name an existing connection under a
	// workspace or organization that does not own it.
	WrongWorkspace credentials.ConnectionRef
	WrongOrg       credentials.ConnectionRef
	// Missing resolves to nothing.
	Missing credentials.ConnectionRef
	// Undecryptable exists but its secrets cannot be opened.
	Undecryptable credentials.ConnectionRef
	// Secrets must never appear in error text, formatted output or logs.
	Secrets []string
}

// RunProviderContract runs the provider contract suite.
func RunProviderContract(t *testing.T, provider credentials.Provider, fx Fixture) {
	t.Helper()

	t.Run("resolves", func(t *testing.T) {
		got, err := provider.Resolve(context.Background(), fx.Ref)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.DSN != fx.WantDSN {
			t.Fatalf("DSN mismatch")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := provider.Resolve(context.Background(), fx.Missing)
		if !errors.Is(err, credentials.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	for name, ref := range map[string]credentials.ConnectionRef{
		"wrong workspace": fx.WrongWorkspace,
		"wrong org":       fx.WrongOrg,
	} {
		t.Run(name+" is not found", func(t *testing.T) {
			_, err := provider.Resolve(context.Background(), ref)
			if !errors.Is(err, credentials.ErrNotFound) {
				t.Fatalf("err = %v, want ErrNotFound", err)
			}
		})
	}

	t.Run("decrypt failure is not not-found and leaks nothing", func(t *testing.T) {
		_, err := provider.Resolve(context.Background(), fx.Undecryptable)
		if err == nil {
			t.Fatal("expected error")
		}
		if errors.Is(err, credentials.ErrNotFound) {
			t.Fatalf("decrypt failure reported as not found")
		}
		assertNoSecrets(t, "error", err.Error(), fx.Secrets)
	})

	for name, ref := range map[string]credentials.ConnectionRef{"plain": fx.Ref, "ssh": fx.WithSSH} {
		t.Run("credentials never render secrets/"+name, func(t *testing.T) {
			got, err := provider.Resolve(context.Background(), ref)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
				assertNoSecrets(t, verb, fmt.Sprintf(verb, got), fx.Secrets)
				assertNoSecrets(t, verb+" ptr", fmt.Sprintf(verb, &got), fx.Secrets)
				if got.SSH != nil {
					assertNoSecrets(t, verb+" ssh", fmt.Sprintf(verb, got.SSH), fx.Secrets)
					assertNoSecrets(t, verb+" ssh value", fmt.Sprintf(verb, *got.SSH), fx.Secrets)
				}
			}
			for _, h := range []func(*bytes.Buffer) slog.Handler{
				func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
				func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
			} {
				var buf bytes.Buffer
				slog.New(h(&buf)).Info("resolved", "creds", got, "ssh", got.SSH)
				assertNoSecrets(t, "slog", buf.String(), fx.Secrets)
			}
			if b, err := json.Marshal(got); err == nil {
				t.Fatalf("json.Marshal(Credentials) succeeded: %d bytes", len(b))
			}
			if got.SSH != nil {
				if b, err := json.Marshal(got.SSH); err == nil {
					t.Fatalf("json.Marshal(SSH) succeeded: %d bytes", len(b))
				}
			}
		})
	}
}

func assertNoSecrets(t *testing.T, what, text string, secrets []string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(text, s) {
			t.Fatalf("%s leaks a secret", what)
		}
	}
}
