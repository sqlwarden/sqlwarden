package web

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/encrypt"
)

// seedEncryptedFileContent writes keyring-encrypted bytes to the active file
// store and records an application-encrypted content row pointing at them.
func seedEncryptedFileContent(t *testing.T, app *application, ws database.Workspace, kr *encrypt.Keyring, plaintext string) (database.WorkspaceFileContent, string) {
	t.Helper()
	ctx := context.Background()

	file := database.WorkspaceFile{
		WorkspaceID: ws.ID,
		Visibility:  database.FileVisibilityShared,
		ObjectType:  database.FileObjectTypeFile,
		Name:        "secret.sql",
		CreatedBy:   1,
		UpdatedBy:   1,
	}
	if err := app.db.InsertWorkspaceFile(ctx, &file); err != nil {
		t.Fatal(err)
	}

	ciphertext, err := kr.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}

	backendID := app.fileStores.ActiveBackendID()
	store, err := app.fileStores.Store(ctx, backendID)
	if err != nil {
		t.Fatal(err)
	}
	storageKey := "objects/" + ws.Name + "/secret"
	object, err := store.Put(ctx, storageKey, strings.NewReader(ciphertext))
	if err != nil {
		t.Fatal(err)
	}

	saved, err := app.db.SaveWorkspaceFileContent(ctx, file.ID, 1, database.WorkspaceFileContent{
		StorageBackendID:     backendID,
		StorageKey:           object.Key,
		ContentHash:          object.ContentHash,
		SizeBytes:            object.SizeBytes,
		ApplicationEncrypted: true,
		EncryptionKeyID:      kr.PrimaryKeyID(),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	return saved, storageKey
}

func TestRotateEncryptionKeys(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)

	// Keyring with a fresh primary and the retired key kept for decryption.
	keyring, err := encrypt.NewKeyring("new-primary-key", "old-retired-key")
	if err != nil {
		t.Fatal(err)
	}
	app.keyring = keyring

	oldKeyring, err := encrypt.NewKeyring("old-retired-key")
	if err != nil {
		t.Fatal(err)
	}

	account, _, org := seedOrgOwner(t, app, "owner@example.com", "Owner", "Rotate Org")
	ws := seedWorkspaceForAccount(t, app, org, account, "Rotate WS", "")
	env := seedEnvironment(t, app, ws.ID, org.ID, "prod")

	// A connection whose DSN is sealed with the now-retired key.
	staleDSN := "postgres://user:pass@db:5432/app"
	staleCipher, err := oldKeyring.Encrypt(staleDSN)
	if err != nil {
		t.Fatal(err)
	}
	staleConn, err := app.db.InsertConnection(ctx, ws.ID, &env.ID, "stale", "postgres", staleCipher, "open")
	if err != nil {
		t.Fatal(err)
	}

	// A legacy, untagged DSN encrypted directly with the current primary key.
	legacyDSN := "mysql://root:secret@db:3306/legacy"
	legacyCipher, err := encrypt.Encrypt(encrypt.DeriveKey("new-primary-key"), legacyDSN)
	if err != nil {
		t.Fatal(err)
	}
	legacyConn, err := app.db.InsertConnection(ctx, ws.ID, &env.ID, "legacy", "mysql", legacyCipher, "open")
	if err != nil {
		t.Fatal(err)
	}

	// A connection already sealed with the primary key — must be left untouched.
	freshDSN := "sqlite:///data/fresh.db"
	freshCipher, err := keyring.Encrypt(freshDSN)
	if err != nil {
		t.Fatal(err)
	}
	freshConn, err := app.db.InsertConnection(ctx, ws.ID, &env.ID, "fresh", "sqlite", freshCipher, "open")
	if err != nil {
		t.Fatal(err)
	}

	// An application-encrypted file content row sealed with the retired key.
	fileContent, storageKey := seedEncryptedFileContent(t, app, ws, oldKeyring, "select 1;")

	report, err := app.RotateEncryptionKeys(ctx)
	if err != nil {
		t.Fatalf("rotateEncryptionKeys failed: %v", err)
	}

	if report.ConnectionsScanned != 3 {
		t.Errorf("expected 3 connections scanned, got %d", report.ConnectionsScanned)
	}
	if report.ConnectionsRotated != 2 {
		t.Errorf("expected 2 connections rotated, got %d", report.ConnectionsRotated)
	}
	if report.FileContentsScanned != 1 {
		t.Errorf("expected 1 file content scanned, got %d", report.FileContentsScanned)
	}
	if report.FileContentsRotated != 1 {
		t.Errorf("expected 1 file content rotated, got %d", report.FileContentsRotated)
	}

	// Stale connection is now sealed with the primary key and still decrypts.
	got, found, err := app.db.GetConnection(ctx, staleConn.ID)
	if err != nil || !found {
		t.Fatalf("reload stale connection: found=%v err=%v", found, err)
	}
	if app.keyring.NeedsRotation(got.DSNEncrypted) {
		t.Error("stale connection DSN still needs rotation after rotate")
	}
	if plain, err := app.keyring.Decrypt(got.DSNEncrypted); err != nil || plain != staleDSN {
		t.Errorf("stale DSN decrypt = %q, %v; want %q", plain, err, staleDSN)
	}

	// Legacy untagged connection is upgraded to the tagged primary format.
	got, _, err = app.db.GetConnection(ctx, legacyConn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if app.keyring.NeedsRotation(got.DSNEncrypted) {
		t.Error("legacy connection DSN still needs rotation after rotate")
	}
	if plain, err := app.keyring.Decrypt(got.DSNEncrypted); err != nil || plain != legacyDSN {
		t.Errorf("legacy DSN decrypt = %q, %v; want %q", plain, err, legacyDSN)
	}

	// Already-current connection is unchanged.
	got, _, err = app.db.GetConnection(ctx, freshConn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DSNEncrypted != freshCipher {
		t.Error("fresh connection DSN was rewritten unnecessarily")
	}

	// File content re-keyed: bytes decrypt with the primary key and the row's
	// key id reflects the new primary.
	store, err := app.fileStores.Store(ctx, fileContent.StorageBackendID)
	if err != nil {
		t.Fatal(err)
	}
	reader, _, err := store.Get(ctx, storageKey)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if app.keyring.NeedsRotation(string(raw)) {
		t.Error("file content bytes still need rotation after rotate")
	}
	if plain, err := app.keyring.Decrypt(string(raw)); err != nil || plain != "select 1;" {
		t.Errorf("file content decrypt = %q, %v; want %q", plain, err, "select 1;")
	}
	reloaded, found, err := app.db.GetWorkspaceFileContent(ctx, fileContent.ID)
	if err != nil || !found {
		t.Fatalf("reload file content: found=%v err=%v", found, err)
	}
	if reloaded.EncryptionKeyID != app.keyring.PrimaryKeyID() {
		t.Errorf("file content key id = %q; want primary %q", reloaded.EncryptionKeyID, app.keyring.PrimaryKeyID())
	}
	if reloaded.ContentHash == fileContent.ContentHash {
		t.Error("file content hash was not updated after re-encryption")
	}

	// Running rotation again is a no-op now that everything is current.
	report, err = app.RotateEncryptionKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConnectionsRotated != 0 || report.FileContentsRotated != 0 {
		t.Errorf("second rotation rotated %d connections, %d file contents; want 0/0",
			report.ConnectionsRotated, report.FileContentsRotated)
	}
}

func TestRotateEncryptionKeysEndpointRequiresAuth(t *testing.T) {
	app := newTestApp(t)

	res := send(t, newTestRequest(t, http.MethodPost, "/api/v1/instance/encryption/rotate", nil), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusUnauthorized)
}

func TestRotateEncryptionKeysEndpointForbidsNonAdmin(t *testing.T) {
	app := newTestApp(t)
	setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	regRes := registerTestUser(t, app, "regular@example.com", "Regular", "securepass99")
	assert.Equal(t, regRes.StatusCode, http.StatusCreated)
	loginRes := loginTestUser(t, app, "regular@example.com", "securepass99")
	tok := extractAccessToken(t, loginRes)

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/encryption/rotate", nil, tok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusForbidden)
}

func TestRotateEncryptionKeysEndpointReturnsReport(t *testing.T) {
	app := newTestApp(t)
	adminTok := setupInstance(t, app, "admin@example.com", "Admin", "securepass99")

	res := send(t, newAuthRequest(t, http.MethodPost, "/api/v1/instance/encryption/rotate", nil, adminTok), app.routes())
	assert.Equal(t, res.StatusCode, http.StatusOK)
	if _, ok := res.BodyFields["connections_scanned"]; !ok {
		t.Errorf("expected connections_scanned in response, got %v", res.BodyFields)
	}
	if _, ok := res.BodyFields["file_contents_rotated"]; !ok {
		t.Errorf("expected file_contents_rotated in response, got %v", res.BodyFields)
	}
}
