package encrypt

import "testing"

func TestKeyringDecryptsK1CiphertextAndMarksItForRotation(t *testing.T) {
	const passphrase = "legacy-passphrase"
	legacyKey := deriveLegacyKey(passphrase)
	payload, err := Encrypt(legacyKey, "legacy-secret")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	ciphertext := legacyEnvelopeVersion + "." + keyID(legacyKey) + "." + payload

	kr, err := NewKeyring(passphrase)
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	plaintext, err := kr.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if plaintext != "legacy-secret" {
		t.Fatalf("Decrypt returned %q, want %q", plaintext, "legacy-secret")
	}
	if !kr.NeedsRotation(ciphertext) {
		t.Fatal("k1 ciphertext should require rotation")
	}
}

func TestKeyringDecryptsLegacyUntaggedCiphertext(t *testing.T) {
	const passphrase = "legacy-passphrase"
	payload, err := Encrypt(deriveLegacyKey(passphrase), "legacy-secret")
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	kr, err := NewKeyring(passphrase)
	if err != nil {
		t.Fatalf("NewKeyring failed: %v", err)
	}
	plaintext, err := kr.Decrypt(payload)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if plaintext != "legacy-secret" {
		t.Fatalf("Decrypt returned %q, want %q", plaintext, "legacy-secret")
	}
}
