package encrypt_test

import (
	"testing"

	"github.com/sqlwarden/internal/encrypt"
)

func TestRoundTrip(t *testing.T) {
	key := encrypt.DeriveKey("test-passphrase")
	plaintext := "hello, world"

	ciphertext, err := encrypt.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := encrypt.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("expected %q, got %q", plaintext, got)
	}
}

func TestDifferentCiphertextEachCall(t *testing.T) {
	key := encrypt.DeriveKey("test-passphrase")
	plaintext := "hello, world"

	ct1, err := encrypt.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("first Encrypt failed: %v", err)
	}

	ct2, err := encrypt.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("second Encrypt failed: %v", err)
	}

	if ct1 == ct2 {
		t.Error("expected different ciphertexts for each call, got identical")
	}
}

func TestWrongKeyReturnsError(t *testing.T) {
	key := encrypt.DeriveKey("correct-passphrase")
	wrongKey := encrypt.DeriveKey("wrong-passphrase")
	plaintext := "sensitive data"

	ciphertext, err := encrypt.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = encrypt.Decrypt(wrongKey, ciphertext)
	if err == nil {
		t.Error("expected error decrypting with wrong key, got nil")
	}
}

func TestTamperedCiphertextReturnsError(t *testing.T) {
	key := encrypt.DeriveKey("test-passphrase")
	plaintext := "tamper test"

	ciphertext, err := encrypt.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Flip a byte in the middle of the ciphertext string
	tampered := []byte(ciphertext)
	mid := len(tampered) / 2
	tampered[mid] ^= 0xFF

	_, err = encrypt.Decrypt(key, string(tampered))
	if err == nil {
		t.Error("expected error decrypting tampered ciphertext, got nil")
	}
}

func TestEmptyPlaintextRoundTrips(t *testing.T) {
	key := encrypt.DeriveKey("test-passphrase")
	plaintext := ""

	ciphertext, err := encrypt.Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	got, err := encrypt.Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if got != plaintext {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestKeyLengthValidation(t *testing.T) {
	shortKey := []byte("too-short")

	_, err := encrypt.Encrypt(shortKey, "plaintext")
	if err == nil {
		t.Error("expected error with non-32-byte key, got nil")
	}

	_, err = encrypt.Decrypt(shortKey, "ciphertext")
	if err == nil {
		t.Error("expected error with non-32-byte key for Decrypt, got nil")
	}
}
