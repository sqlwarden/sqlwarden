package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// DeriveKey stretches a passphrase to a 32-byte AES-256 key using SHA-256.
func DeriveKey(passphrase string) []byte {
	h := sha256.Sum256([]byte(passphrase))
	return h[:]
}

// Encrypt encrypts plaintext with AES-256-GCM.
// Returns base64url-encoded ciphertext (nonce prepended to ciphertext).
// Each call produces a different ciphertext due to a random nonce.
func Encrypt(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("encrypt: key must be 32 bytes")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("encrypt: failed to generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a value produced by Encrypt.
// Returns an error if the key is wrong or the ciphertext is tampered.
func Decrypt(key []byte, ciphertext string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("encrypt: key must be 32 bytes")
	}

	data, err := base64.URLEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to decode ciphertext: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("encrypt: failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("encrypt: ciphertext too short")
	}

	nonce, data := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", fmt.Errorf("encrypt: authentication failed: %w", err)
	}

	return string(plaintext), nil
}
