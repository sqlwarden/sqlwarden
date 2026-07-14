package encrypt

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const envelopeVersion = "k2"

// Keyring holds the active encryption key plus any retired keys kept around so
// ciphertext produced with previous configured keys stays decryptable through
// a rotation.
//
// New ciphertext is always sealed with the primary key and tagged with a key id
// so it can be routed to the right key on decryption. NeedsRotation reports
// whether a value should be re-encrypted with the current primary key.
type Keyring struct {
	primaryID string
	keys      map[string][]byte // key id -> 32-byte key material
}

// NewKeyring builds a keyring from a primary passphrase and zero or more
// previous passphrases retained for decryption during rotation. Passphrases are
// derived with DeriveKey. Duplicate passphrases (including a previous key that
// equals the primary) are deduplicated. The primary passphrase must not be empty.
func NewKeyring(primary string, previous ...string) (*Keyring, error) {
	if primary == "" {
		return nil, errors.New("encrypt: primary key must not be empty")
	}

	kr := &Keyring{keys: make(map[string][]byte)}

	add := func(passphrase string) (string, error) {
		key, err := DeriveKey(passphrase)
		if err != nil {
			return "", err
		}
		id := keyID(key)
		if _, exists := kr.keys[id]; !exists {
			kr.keys[id] = key
		}
		return id, nil
	}

	var err error
	kr.primaryID, err = add(primary)
	if err != nil {
		return nil, fmt.Errorf("encrypt: derive primary key: %w", err)
	}
	for _, p := range previous {
		if p == "" {
			continue
		}
		if _, err = add(p); err != nil {
			return nil, fmt.Errorf("encrypt: derive previous key: %w", err)
		}
	}

	return kr, nil
}

// PrimaryKeyID returns the id of the key used to encrypt new values.
func (k *Keyring) PrimaryKeyID() string {
	return k.primaryID
}

// Encrypt seals plaintext with the primary key and returns a tagged ciphertext
// of the form "k2.<keyID>.<base64payload>".
func (k *Keyring) Encrypt(plaintext string) (string, error) {
	payload, err := Encrypt(k.keys[k.primaryID], plaintext)
	if err != nil {
		return "", err
	}
	return envelopeVersion + "." + k.primaryID + "." + payload, nil
}

// Decrypt decrypts a k2 value produced by Encrypt. Values in older or untagged
// formats are intentionally unsupported.
func (k *Keyring) Decrypt(ciphertext string) (string, error) {
	id, payload, ok := parseEnvelope(ciphertext)
	if !ok {
		return "", errors.New("encrypt: unsupported ciphertext format")
	}
	key, ok := k.keys[id]
	if !ok {
		return "", fmt.Errorf("encrypt: no key for id %q", id)
	}
	return Decrypt(key, payload)
}

// NeedsRotation reports whether ciphertext should be re-encrypted with the
// current primary key. Unsupported formats and values tagged with any key other
// than the primary require rotation.
func (k *Keyring) NeedsRotation(ciphertext string) bool {
	id, _, ok := parseEnvelope(ciphertext)
	if !ok {
		return true
	}
	return id != k.primaryID
}

// keyID derives a short, stable, non-reversible fingerprint of a key. It hashes
// the (already hashed) key material and truncates, so it leaks nothing about the
// underlying passphrase while remaining deterministic across processes.
func keyID(key []byte) string {
	sum := sha256.Sum256(key)
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}

// parseEnvelope splits a k2 ciphertext into its key id and payload.
func parseEnvelope(ciphertext string) (id, payload string, ok bool) {
	parts := strings.SplitN(ciphertext, ".", 3)
	if len(parts) != 3 || parts[0] != envelopeVersion {
		return "", "", false
	}
	return parts[1], parts[2], true
}
