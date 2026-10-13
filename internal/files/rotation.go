package files

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/sqlwarden/internal/database"
)

// ContentKeyring is the keyring surface content rotation opens and seals with.
type ContentKeyring interface {
	Decrypt(ciphertext string) (string, error)
	Encrypt(plaintext string) (string, error)
	NeedsRotation(ciphertext string) bool
	PrimaryKeyID() string
}

// ContentRotator re-seals application-encrypted file contents with the
// keyring's primary key, rewriting each to its existing storage key. It runs
// as an operator task and takes no principal; keyrotation authorizes callers.
type ContentRotator struct {
	db     *database.DB
	stores StoreResolver
	keys   ContentKeyring
}

func NewContentRotator(db *database.DB, stores StoreResolver, keys ContentKeyring) *ContentRotator {
	return &ContentRotator{db: db, stores: stores, keys: keys}
}

// RotateContents returns how many contents were scanned and re-sealed. Each
// content commits on its own, so a failure leaves earlier ones rotated and the
// counts reflect that.
func (r *ContentRotator) RotateContents(ctx context.Context) (scanned, rotated int, err error) {
	contents, err := r.db.ListApplicationEncryptedFileContents(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("rotate files: list contents: %w", err)
	}
	for _, content := range contents {
		scanned++

		store, err := r.stores.Store(ctx, content.StorageBackendID)
		if err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: resolve backend for content %d: %w", content.ID, err)
		}
		reader, _, err := store.Get(ctx, content.StorageKey)
		if err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: read content %d: %w", content.ID, err)
		}
		raw, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: read content %d: %w", content.ID, err)
		}

		ciphertext := string(raw)
		if !r.keys.NeedsRotation(ciphertext) {
			continue
		}
		plaintext, err := r.keys.Decrypt(ciphertext)
		if err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: decrypt content %d: %w", content.ID, err)
		}
		reencrypted, err := r.keys.Encrypt(plaintext)
		if err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: encrypt content %d: %w", content.ID, err)
		}
		object, err := store.Put(ctx, content.StorageKey, strings.NewReader(reencrypted))
		if err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: write content %d: %w", content.ID, err)
		}
		if err := r.db.UpdateWorkspaceFileContentEncryption(ctx, content.ID, object.ContentHash, r.keys.PrimaryKeyID()); err != nil {
			return scanned, rotated, fmt.Errorf("rotate files: update content %d: %w", content.ID, err)
		}
		rotated++
	}
	return scanned, rotated, nil
}
