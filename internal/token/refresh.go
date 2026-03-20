package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Generate creates a cryptographically random 256-bit opaque token.
// Returns: (plaintextHex, sha256HashHex, error)
func Generate() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("token: generate: %w", err)
	}
	plain := hex.EncodeToString(b)
	return plain, Hash(plain), nil
}

// Hash returns the SHA-256 hex digest of a plaintext token.
func Hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
