package encrypt

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const (
	legacyEnvelopeVersion  = "k1"
	currentEnvelopeVersion = "k2"
)

// Keyring holds the active encryption key plus any retired keys kept around so
// existing ciphertext stays decryptable through a rotation.
//
// New ciphertext is always sealed with the primary key and tagged with a key id
// so it can be routed to the right key on decryption. Legacy ciphertext written
// before key tagging existed has no tag; it is decrypted by trying every known
// key. Either way, NeedsRotation reports whether a value should be re-encrypted
// with the current primary key.
type Keyring struct {
	primaryID string
	keys      map[string][]byte // current key id -> 32-byte key material
	legacy    map[string][]byte // k1 key id -> legacy key material
	all       [][]byte          // current and legacy keys for untagged fallback
}

// NewKeyring builds a keyring from a primary passphrase and zero or more
// previous passphrases retained for decryption during rotation. Passphrases are
// stretched with DeriveKey. Duplicate passphrases (including a previous key that
// equals the primary) are deduplicated. The primary passphrase must not be empty.
func NewKeyring(primary string, previous ...string) (*Keyring, error) {
	if primary == "" {
		return nil, errors.New("encrypt: primary key must not be empty")
	}

	kr := &Keyring{
		keys:   make(map[string][]byte),
		legacy: make(map[string][]byte),
	}

	add := func(passphrase string) (string, error) {
		key, err := DeriveKey(passphrase)
		if err != nil {
			return "", err
		}
		id := keyID(key)
		if _, exists := kr.keys[id]; !exists {
			kr.keys[id] = key
			kr.all = append(kr.all, key)
		}

		legacyKey := deriveLegacyKey(passphrase)
		legacyID := keyID(legacyKey)
		if _, exists := kr.legacy[legacyID]; !exists {
			kr.legacy[legacyID] = legacyKey
			kr.all = append(kr.all, legacyKey)
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
	return currentEnvelopeVersion + "." + k.primaryID + "." + payload, nil
}

// Decrypt decrypts a value produced by Encrypt or by the legacy stateless
// Encrypt function. Tagged values are routed to the key named in the tag; legacy
// untagged values are decrypted by trying every key in the ring.
func (k *Keyring) Decrypt(ciphertext string) (string, error) {
	version, id, payload, tagged := parseEnvelope(ciphertext)
	if tagged {
		keys := k.keys
		if version == legacyEnvelopeVersion {
			keys = k.legacy
		}
		key, ok := keys[id]
		if !ok {
			return "", fmt.Errorf("encrypt: no key for id %q", id)
		}
		return Decrypt(key, payload)
	}

	// Legacy untagged ciphertext: try each key until one authenticates.
	var lastErr error
	for _, key := range k.all {
		plaintext, err := Decrypt(key, ciphertext)
		if err == nil {
			return plaintext, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("encrypt: no keys available")
	}
	return "", fmt.Errorf("encrypt: legacy decrypt failed: %w", lastErr)
}

// NeedsRotation reports whether ciphertext should be re-encrypted with the
// current primary key. It is true for legacy untagged values and for values
// tagged with any key other than the primary.
func (k *Keyring) NeedsRotation(ciphertext string) bool {
	version, id, _, tagged := parseEnvelope(ciphertext)
	if !tagged {
		return true
	}
	return version != currentEnvelopeVersion || id != k.primaryID
}

// keyID derives a short, stable, non-reversible fingerprint of a key. It hashes
// the (already hashed) key material and truncates, so it leaks nothing about the
// underlying passphrase while remaining deterministic across processes.
func keyID(key []byte) string {
	sum := sha256.Sum256(key)
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}

// parseEnvelope splits a tagged ciphertext into its key id and payload. It
// returns tagged=false for anything that is not in the "k1.<id>.<payload>"
// format, which is treated as legacy untagged ciphertext.
func parseEnvelope(ciphertext string) (version, id, payload string, tagged bool) {
	parts := strings.SplitN(ciphertext, ".", 3)
	if len(parts) != 3 || (parts[0] != legacyEnvelopeVersion && parts[0] != currentEnvelopeVersion) {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
