package encrypt_test

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/encrypt"
)

func TestKeyringRoundTrip(t *testing.T) {
	kr, err := encrypt.NewKeyring("primary-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	plaintext := "postgres://user:pass@host:5432/db"

	ciphertext, err := kr.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := kr.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("expected %q, got %q", plaintext, got)
	}
}

func TestKeyringEmptyPrimaryRejected(t *testing.T) {
	if _, err := encrypt.NewKeyring(""); err == nil {
		t.Error("expected error for empty primary passphrase, got nil")
	}
}

func TestKeyringTaggedFormat(t *testing.T) {
	kr, err := encrypt.NewKeyring("primary-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	ciphertext, err := kr.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	parts := strings.Split(ciphertext, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 dot-separated parts, got %d (%q)", len(parts), ciphertext)
	}
	if parts[0] != "k1" {
		t.Errorf("expected envelope version %q, got %q", "k1", parts[0])
	}
	if parts[1] != kr.PrimaryKeyID() {
		t.Errorf("expected key id %q, got %q", kr.PrimaryKeyID(), parts[1])
	}
}

func TestKeyringKeyIDDeterministic(t *testing.T) {
	kr1, err := encrypt.NewKeyring("same-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	kr2, err := encrypt.NewKeyring("same-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	if kr1.PrimaryKeyID() != kr2.PrimaryKeyID() {
		t.Errorf("expected identical key ids for the same passphrase, got %q and %q",
			kr1.PrimaryKeyID(), kr2.PrimaryKeyID())
	}

	krOther, err := encrypt.NewKeyring("different-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	if kr1.PrimaryKeyID() == krOther.PrimaryKeyID() {
		t.Error("expected different key ids for different passphrases, got identical")
	}
}

func TestKeyringDecryptsLegacyUntagged(t *testing.T) {
	// A value produced by the old stateless Encrypt (no envelope tag).
	key := encrypt.DeriveKey("primary-passphrase")
	legacy, err := encrypt.Encrypt(key, "legacy-secret")
	if err != nil {
		t.Fatalf("legacy Encrypt failed: %v", err)
	}

	kr, err := encrypt.NewKeyring("primary-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	got, err := kr.Decrypt(legacy)
	if err != nil {
		t.Fatalf("Decrypt of legacy value failed: %v", err)
	}
	if got != "legacy-secret" {
		t.Errorf("expected %q, got %q", "legacy-secret", got)
	}
}

func TestKeyringDecryptsWithPreviousKey(t *testing.T) {
	// Encrypt with an old keyring whose primary later becomes a previous key.
	oldKR, err := encrypt.NewKeyring("old-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	ciphertext, err := oldKR.Encrypt("rotate-me")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// New keyring: new primary, old key retained as previous.
	newKR, err := encrypt.NewKeyring("new-passphrase", "old-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	got, err := newKR.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt with previous key failed: %v", err)
	}
	if got != "rotate-me" {
		t.Errorf("expected %q, got %q", "rotate-me", got)
	}
}

func TestKeyringUnknownKeyIDReturnsError(t *testing.T) {
	strangerKR, err := encrypt.NewKeyring("stranger-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	ciphertext, err := strangerKR.Encrypt("not-yours")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	kr, err := encrypt.NewKeyring("primary-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	if _, err := kr.Decrypt(ciphertext); err == nil {
		t.Error("expected error decrypting value tagged with an unknown key id, got nil")
	}
}

func TestKeyringNeedsRotation(t *testing.T) {
	kr, err := encrypt.NewKeyring("new-passphrase", "old-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	// Tagged with the primary key — up to date.
	primary, err := kr.Encrypt("fresh")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if kr.NeedsRotation(primary) {
		t.Error("value tagged with primary key should not need rotation")
	}

	// Tagged with a previous key — needs rotation.
	oldKR, err := encrypt.NewKeyring("old-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	previous, err := oldKR.Encrypt("stale")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if !kr.NeedsRotation(previous) {
		t.Error("value tagged with a previous key should need rotation")
	}

	// Legacy untagged — needs rotation.
	legacy, err := encrypt.Encrypt(encrypt.DeriveKey("new-passphrase"), "legacy")
	if err != nil {
		t.Fatalf("legacy Encrypt failed: %v", err)
	}
	if !kr.NeedsRotation(legacy) {
		t.Error("legacy untagged value should need rotation")
	}
}

func TestKeyringReEncryptClearsRotation(t *testing.T) {
	kr, err := encrypt.NewKeyring("new-passphrase", "old-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}

	oldKR, err := encrypt.NewKeyring("old-passphrase")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	stale, err := oldKR.Encrypt("payload")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if !kr.NeedsRotation(stale) {
		t.Fatal("precondition: stale value should need rotation")
	}

	plaintext, err := kr.Decrypt(stale)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	rotated, err := kr.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("re-Encrypt failed: %v", err)
	}

	if kr.NeedsRotation(rotated) {
		t.Error("re-encrypted value should not need rotation")
	}
	got, err := kr.Decrypt(rotated)
	if err != nil {
		t.Fatalf("Decrypt of rotated value failed: %v", err)
	}
	if got != "payload" {
		t.Errorf("expected %q, got %q", "payload", got)
	}
}

func TestKeyringDuplicatePassphraseDeduped(t *testing.T) {
	// Passing the primary again as a previous key must not break decryption.
	kr, err := encrypt.NewKeyring("primary", "primary")
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	ciphertext, err := kr.Encrypt("data")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	got, err := kr.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if got != "data" {
		t.Errorf("expected %q, got %q", "data", got)
	}
}
