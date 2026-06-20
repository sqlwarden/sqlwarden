package web

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sqlwarden/internal/response"
)

// EncryptionRotationReport summarizes a key-rotation pass over all
// application-encrypted data.
type EncryptionRotationReport struct {
	ConnectionsScanned  int `json:"connections_scanned"`
	ConnectionsRotated  int `json:"connections_rotated"`
	FileContentsScanned int `json:"file_contents_scanned"`
	FileContentsRotated int `json:"file_contents_rotated"`
}

// rotateEncryptionKeysHandler re-encrypts all application-encrypted data with
// the current primary key. It is an instance-admin-only operation and returns a
// summary of what was scanned and rotated.
func (app *application) rotateEncryptionKeysHandler(w http.ResponseWriter, r *http.Request) {
	report, err := app.RotateEncryptionKeys(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := response.JSON(w, http.StatusOK, report); err != nil {
		app.serverError(w, r, err)
	}
}

// RotateEncryptionKeys re-encrypts every piece of application-encrypted data
// that is not already sealed with the keyring's primary key. It decrypts each
// value through the keyring (which still holds retired keys) and re-encrypts it
// with the current primary key.
//
// Rotation is idempotent: values already sealed with the primary key are left
// untouched, so it is safe to run repeatedly. Each item is committed
// independently, so a mid-run failure leaves already-rotated items rotated.
func (app *application) RotateEncryptionKeys(ctx context.Context) (EncryptionRotationReport, error) {
	var report EncryptionRotationReport

	app.logger.InfoContext(ctx, "encryption key rotation started")
	if err := app.rotateConnectionDSNs(ctx, &report); err != nil {
		return report, err
	}
	if err := app.rotateFileContents(ctx, &report); err != nil {
		return report, err
	}
	app.logger.InfoContext(ctx, "encryption key rotation complete",
		slog.Int("connections_scanned", report.ConnectionsScanned),
		slog.Int("connections_rotated", report.ConnectionsRotated),
		slog.Int("file_contents_scanned", report.FileContentsScanned),
		slog.Int("file_contents_rotated", report.FileContentsRotated),
	)
	return report, nil
}

// rotateConnectionDSNs re-encrypts stored connection DSNs with the primary key.
func (app *application) rotateConnectionDSNs(ctx context.Context, report *EncryptionRotationReport) error {
	conns, err := app.db.ListAllConnections(ctx)
	if err != nil {
		return fmt.Errorf("rotate dsn: list connections: %w", err)
	}
	for _, conn := range conns {
		report.ConnectionsScanned++
		if !app.keyring.NeedsRotation(conn.DSNEncrypted) {
			continue
		}
		plaintext, err := app.keyring.Decrypt(conn.DSNEncrypted)
		if err != nil {
			return fmt.Errorf("rotate dsn: decrypt connection %d: %w", conn.ID, err)
		}
		reencrypted, err := app.keyring.Encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("rotate dsn: encrypt connection %d: %w", conn.ID, err)
		}
		if err := app.db.UpdateConnectionDSN(ctx, conn.ID, reencrypted); err != nil {
			return fmt.Errorf("rotate dsn: update connection %d: %w", conn.ID, err)
		}
		report.ConnectionsRotated++
	}
	app.logger.InfoContext(ctx, "connection dsn rotation pass complete",
		slog.Int("connections_scanned", report.ConnectionsScanned),
		slog.Int("connections_rotated", report.ConnectionsRotated),
	)
	return nil
}

// rotateFileContents re-encrypts application-encrypted file content bytes in
// place, rewriting them to the same storage key under the primary key.
func (app *application) rotateFileContents(ctx context.Context, report *EncryptionRotationReport) error {
	contents, err := app.db.ListApplicationEncryptedFileContents(ctx)
	if err != nil {
		return fmt.Errorf("rotate files: list contents: %w", err)
	}
	for _, content := range contents {
		report.FileContentsScanned++

		store, err := app.fileStores.Store(ctx, content.StorageBackendID)
		if err != nil {
			return fmt.Errorf("rotate files: resolve backend for content %d: %w", content.ID, err)
		}
		reader, _, err := store.Get(ctx, content.StorageKey)
		if err != nil {
			return fmt.Errorf("rotate files: read content %d: %w", content.ID, err)
		}
		raw, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			return fmt.Errorf("rotate files: read content %d: %w", content.ID, err)
		}

		ciphertext := string(raw)
		if !app.keyring.NeedsRotation(ciphertext) {
			continue
		}
		plaintext, err := app.keyring.Decrypt(ciphertext)
		if err != nil {
			return fmt.Errorf("rotate files: decrypt content %d: %w", content.ID, err)
		}
		reencrypted, err := app.keyring.Encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("rotate files: encrypt content %d: %w", content.ID, err)
		}
		object, err := store.Put(ctx, content.StorageKey, strings.NewReader(reencrypted))
		if err != nil {
			return fmt.Errorf("rotate files: write content %d: %w", content.ID, err)
		}
		if err := app.db.UpdateWorkspaceFileContentEncryption(ctx, content.ID, object.ContentHash, app.keyring.PrimaryKeyID()); err != nil {
			return fmt.Errorf("rotate files: update content %d: %w", content.ID, err)
		}
		report.FileContentsRotated++
	}
	app.logger.InfoContext(ctx, "file content encryption rotation pass complete",
		slog.Int("file_contents_scanned", report.FileContentsScanned),
		slog.Int("file_contents_rotated", report.FileContentsRotated),
	)
	return nil
}
