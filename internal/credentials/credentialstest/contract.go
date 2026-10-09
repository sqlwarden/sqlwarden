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
	// UndecryptableMention, when set, must appear in the Undecryptable error
	// text so operators can identify the failing connection.
	UndecryptableMention string
	// Reference identifies a connection whose ReferenceName is externally
	// managed and therefore cannot be revealed.
	Reference     credentials.ConnectionRef
	ReferenceName credentials.SecretName
	// StoredName and StoredValue identify a stored value that Reveal returns.
	StoredName  credentials.SecretName
	StoredValue string
	// WantStates are asserted against Describe(Ref).
	WantStates map[credentials.SecretName]credentials.SecretState
	// Secrets must never appear in error text, formatted output or logs.
	Secrets []string
}

// RunProviderContract runs the provider contract suite.
func RunProviderContract(t *testing.T, provider credentials.Resolver, fx Fixture) {
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
		"wrong workspace":      fx.WrongWorkspace,
		"wrong org":            fx.WrongOrg,
		"malformed org":        withOrg(fx.Ref, "not-an-id"),
		"malformed workspace":  withWorkspace(fx.Ref, "not-an-id"),
		"malformed connection": withConnection(fx.Ref, "not-an-id"),
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
		if fx.UndecryptableMention != "" && !strings.Contains(err.Error(), fx.UndecryptableMention) {
			t.Fatalf("decrypt failure error does not identify the connection")
		}
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

	full, ok := provider.(credentials.Provider)
	if !ok {
		t.Log("provider does not support Describe or Reveal; extended contract skipped")
		return
	}
	runExtendedProviderContract(t, full, fx)
}

func runExtendedProviderContract(t *testing.T, provider credentials.Provider, fx Fixture) {
	t.Helper()

	for name, ref := range map[string]credentials.ConnectionRef{
		"missing":              fx.Missing,
		"wrong workspace":      fx.WrongWorkspace,
		"wrong org":            fx.WrongOrg,
		"malformed org":        withOrg(fx.Ref, "not-an-id"),
		"malformed workspace":  withWorkspace(fx.Ref, "not-an-id"),
		"malformed connection": withConnection(fx.Ref, "not-an-id"),
	} {
		t.Run("describe and reveal "+name+" as not found", func(t *testing.T) {
			if _, err := provider.Describe(context.Background(), ref); !errors.Is(err, credentials.ErrNotFound) {
				t.Fatalf("Describe error = %v, want ErrNotFound", err)
			}
			if _, err := provider.Reveal(context.Background(), ref, credentials.SecretPassword); !errors.Is(err, credentials.ErrNotFound) {
				t.Fatalf("Reveal error = %v, want ErrNotFound", err)
			}
		})
	}

	t.Run("describe reports safe states", func(t *testing.T) {
		states, err := provider.Describe(context.Background(), fx.Ref)
		if err != nil {
			t.Fatalf("Describe: %v", err)
		}
		for name, want := range fx.WantStates {
			if got := states[name]; got != want {
				t.Fatalf("state %q = %+v, want %+v", name, got, want)
			}
		}
		encoded, err := json.Marshal(states)
		if err != nil {
			t.Fatalf("marshal states: %v", err)
		}
		assertNoSecrets(t, "Describe JSON", string(encoded), fx.Secrets)
	})

	t.Run("reveal stored", func(t *testing.T) {
		got, err := provider.Reveal(context.Background(), fx.Ref, fx.StoredName)
		if err != nil {
			t.Fatalf("Reveal: %v", err)
		}
		if got != fx.StoredValue {
			t.Fatal("revealed value mismatch")
		}
	})

	t.Run("reference is not revealable", func(t *testing.T) {
		states, err := provider.Describe(context.Background(), fx.Reference)
		if err != nil {
			t.Fatalf("Describe reference: %v", err)
		}
		if got := states[fx.ReferenceName]; got != (credentials.SecretState{Set: true, Source: credentials.SourceReference}) {
			t.Fatalf("reference state = %+v", got)
		}
		_, err = provider.Reveal(context.Background(), fx.Reference, fx.ReferenceName)
		if !errors.Is(err, credentials.ErrNotRevealable) {
			t.Fatalf("err = %v, want ErrNotRevealable", err)
		}
	})

	t.Run("unset is not revealable", func(t *testing.T) {
		_, err := provider.Reveal(context.Background(), fx.Ref, credentials.SecretTLSClientKey)
		if !errors.Is(err, credentials.ErrNotRevealable) {
			t.Fatalf("err = %v, want ErrNotRevealable", err)
		}
	})

	t.Run("operations do not log secrets", func(t *testing.T) {
		var buf bytes.Buffer
		previous := slog.Default()
		slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(previous) })

		_, _ = provider.Resolve(context.Background(), fx.Undecryptable)
		_, _ = provider.Describe(context.Background(), fx.Ref)
		_, _ = provider.Reveal(context.Background(), fx.Ref, fx.StoredName)
		_, _ = provider.Reveal(context.Background(), fx.Reference, fx.ReferenceName)
		assertNoSecrets(t, "slog", buf.String(), fx.Secrets)
	})
}

func withOrg(ref credentials.ConnectionRef, value string) credentials.ConnectionRef {
	ref.OrgID = value
	return ref
}

func withWorkspace(ref credentials.ConnectionRef, value string) credentials.ConnectionRef {
	ref.WorkspaceID = value
	return ref
}

func withConnection(ref credentials.ConnectionRef, value string) credentials.ConnectionRef {
	ref.ConnectionID = value
	return ref
}

func assertNoSecrets(t *testing.T, what, text string, secrets []string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(text, s) {
			t.Fatalf("%s leaks a secret", what)
		}
	}
}
