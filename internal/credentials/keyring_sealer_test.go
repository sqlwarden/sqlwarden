package credentials

import (
	"testing"

	"github.com/sqlwarden/internal/encrypt"
)

func TestKeyringSealerRoundTripsAndReportsPrimaryKey(t *testing.T) {
	keyring, err := encrypt.NewKeyring("keyring-sealer-test-key-32-bytes!!")
	if err != nil {
		t.Fatal(err)
	}
	sealer := NewKeyringSealer(keyring)

	ciphertext, keyID, err := sealer.Seal("s3cret-value")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "s3cret-value" || keyID != keyring.PrimaryKeyID() {
		t.Fatalf("ciphertext = %q, key id = %q", ciphertext, keyID)
	}
	got, err := sealer.Open(ciphertext)
	if err != nil || got != "s3cret-value" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, err := sealer.Open("not-a-ciphertext"); err == nil {
		t.Fatal("Open accepted an unsupported ciphertext")
	}
}

func TestKeyringSealerOpensCiphertextSealedWithRetiredKey(t *testing.T) {
	oldKeyring, err := encrypt.NewKeyring("retired-sealer-test-key-32-bytes!")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, oldID, err := NewKeyringSealer(oldKeyring).Seal("legacy-value")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := encrypt.NewKeyring("current-sealer-test-key-32-bytes!", "retired-sealer-test-key-32-bytes!")
	if err != nil {
		t.Fatal(err)
	}
	sealer := NewKeyringSealer(rotated)
	got, err := sealer.Open(ciphertext)
	if err != nil || got != "legacy-value" {
		t.Fatalf("Open = %q, %v", got, err)
	}
	if _, newID, err := sealer.Seal("x"); err != nil || newID == oldID {
		t.Fatalf("new key id = %q (old %q), err = %v", newID, oldID, err)
	}
}
