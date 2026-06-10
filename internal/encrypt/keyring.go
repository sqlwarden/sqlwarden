package encrypt

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// envelopeVersion prefixes every keyring-produced ciphertext. It identifies the
// tagged envelope format (k1) and lets us evolve the layout later without
// ambiguity against legacy untagged ciphertext.
const envelopeVersion = "k1"

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
	keys      map[string][]byte // key id -> 32-byte key material
	all       [][]byte          // every key, for legacy untagged fallback
}

// NewKeyring builds a keyring from a primary passphrase and zero or more
// previous passphrases retained for decryption during rotation. Passphrases are
// stretched with DeriveKey. Duplicate passphrases (including a previous key that
// equals the primary) are deduplicated. The primary passphrase must not be empty.
func NewKeyring(primary string, previous ...string) (*Keyring, error) {
	if primary == "" {
		return nil, errors.New("encrypt: primary key must not be empty")
	}

	kr := &Keyring{keys: make(map[string][]byte)}

	add := func(passphrase string) string {
		key := DeriveKey(passphrase)
		id := keyID(key)
		if _, exists := kr.keys[id]; !exists {
			kr.keys[id] = key
			kr.all = append(kr.all, key)
		}
		return id
	}

	kr.primaryID = add(primary)
	for _, p := range previous {
		if p == "" {
			continue
		}
		add(p)
	}

	return kr, nil
}

// PrimaryKeyID returns the id of the key used to encrypt new values.
func (k *Keyring) PrimaryKeyID() string {
	return k.primaryID
}

// Encrypt seals plaintext with the primary key and returns a tagged ciphertext
// of the form "k1.<keyID>.<base64payload>".
func (k *Keyring) Encrypt(plaintext string) (string, error) {
	payload, err := Encrypt(k.keys[k.primaryID], plaintext)
	if err != nil {
		return "", err
	}
	return envelopeVersion + "." + k.primaryID + "." + payload, nil
}

// Decrypt decrypts a value produced by Encrypt or by the legacy stateless
// Encrypt function. Tagged values are routed to the key named in the tag; legacy
// untagged values are decrypted by trying every key in the ring.
func (k *Keyring) Decrypt(ciphertext string) (string, error) {
	id, payload, tagged := parseEnvelope(ciphertext)
	if tagged {
		key, ok := k.keys[id]
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
	id, _, tagged := parseEnvelope(ciphertext)
	if !tagged {
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

// parseEnvelope splits a tagged ciphertext into its key id and payload. It
// returns tagged=false for anything that is not in the "k1.<id>.<payload>"
// format, which is treated as legacy untagged ciphertext.
func parseEnvelope(ciphertext string) (id, payload string, tagged bool) {
	parts := strings.SplitN(ciphertext, ".", 3)
	if len(parts) != 3 || parts[0] != envelopeVersion {
		return "", "", false
	}
	return parts[1], parts[2], true
}
