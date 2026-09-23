package credentialstest

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/execution"
)

// Contract describes provider fixtures for the shared behavioral checks.
type Contract struct {
	Provider      execution.CredentialProvider
	ExistingID    string
	Expected      execution.Credentials
	MissingID     string
	BrokenID      string
	SecretMarkers []string
	Logs          func() string
}

// RunProviderContract verifies successful resolution, stable not-found and
// decryption errors, and the absence of credential material in errors or logs.
func RunProviderContract(t *testing.T, contract Contract) {
	t.Helper()
	ctx := context.Background()

	t.Run("resolve", func(t *testing.T) {
		resolved, err := contract.Provider.Resolve(ctx, contract.ExistingID)
		if err != nil {
			assertNoSecrets(t, err.Error(), contract.SecretMarkers)
			t.Fatal(err)
		}
		if !reflect.DeepEqual(resolved, contract.Expected) {
			t.Fatal("Resolve() returned unexpected credentials")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := contract.Provider.Resolve(ctx, contract.MissingID)
		if err != nil {
			assertNoSecrets(t, err.Error(), contract.SecretMarkers)
		}
		if !errors.Is(err, execution.ErrCredentialsNotFound) {
			t.Fatalf("Resolve() error = %v, want credentials not found", err)
		}
	})

	t.Run("decryption failure", func(t *testing.T) {
		_, err := contract.Provider.Resolve(ctx, contract.BrokenID)
		if err != nil {
			assertNoSecrets(t, err.Error(), contract.SecretMarkers)
		}
		if !errors.Is(err, execution.ErrCredentialDecryption) {
			t.Fatalf("Resolve() error = %v, want credential decryption failure", err)
		}
	})

	if contract.Logs != nil {
		assertNoSecrets(t, contract.Logs(), contract.SecretMarkers)
	}
}

func assertNoSecrets(t *testing.T, value string, markers []string) {
	t.Helper()
	for _, marker := range markers {
		if marker != "" && strings.Contains(value, marker) {
			t.Fatalf("credential material %q leaked in %q", marker, value)
		}
	}
}
