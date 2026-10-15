package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/assert"
	"github.com/sqlwarden/internal/credentials"
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

func seedLegacyRotationConnection(t *testing.T, app *application, driver, dsn string, kr *encrypt.Keyring) int64 {
	t.Helper()
	ctx := context.Background()
	account, _, org := seedOrgOwner(t, app, "rotation-owner@example.com", "Owner", "Rotation Org")
	workspace := seedWorkspaceForAccount(t, app, org, account, "Rotation WS", "")
	environment := seedEnvironment(t, app, workspace.ID, org.ID, "prod")
	ciphertext, err := kr.Encrypt(dsn)
	if err != nil {
		t.Fatal(err)
	}
	connection, err := app.db.InsertConnection(ctx, workspace.ID, &environment.ID, "legacy", driver, ciphertext, "open")
	if err != nil {
		t.Fatal(err)
	}
	return connection.ID
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

	// A legacy connection already sealed with the primary key still needs to be
	// split into structured fields.
	freshDSN := "file:/data/fresh.db"
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

	if report.ConnectionsSplit != 2 {
		t.Errorf("expected 2 connections split, got %d", report.ConnectionsSplit)
	}
	if report.ConnectionSecretsRotated != 0 {
		t.Errorf("expected split secrets to use the active key, got %d later rotations", report.ConnectionSecretsRotated)
	}
	if report.FileContentsScanned != 1 {
		t.Errorf("expected 1 file content scanned, got %d", report.FileContentsScanned)
	}
	if report.FileContentsRotated != 1 {
		t.Errorf("expected 1 file content rotated, got %d", report.FileContentsRotated)
	}

	// Legacy connections are now structured and their extracted secrets use the
	// active key.
	got, found, err := app.db.GetConnection(ctx, staleConn.ID)
	if err != nil || !found {
		t.Fatalf("reload stale connection: found=%v err=%v", found, err)
	}
	if got.DSNEncrypted != "" {
		t.Error("stale connection DSN was not cleared")
	}
	secrets, err := app.db.ListConnectionSecrets(ctx, staleConn.ID)
	if err != nil || len(secrets) != 1 || secrets[0].Name != "password" {
		t.Fatalf("split secrets = %+v, err=%v", secrets, err)
	}
	if app.keyring.NeedsRotation(secrets[0].ValueEncrypted) {
		t.Error("split password does not use the active key")
	}
	if plain, err := app.keyring.Decrypt(secrets[0].ValueEncrypted); err != nil || plain != "pass" {
		t.Errorf("split password = %q, %v", plain, err)
	}

	// Rows already encrypted with the active key are split too.
	got, _, err = app.db.GetConnection(ctx, freshConn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DSNEncrypted != "" || len(got.Params) == 0 {
		t.Error("fresh legacy connection was not split")
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
	defer reader.Close()
	raw, err := io.ReadAll(reader)
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
	if report.ConnectionsSplit != 0 || report.ConnectionSecretsRotated != 0 || report.FileContentsRotated != 0 {
		t.Errorf("second rotation unexpectedly changed data: %+v", report)
	}
}

func TestRotateEncryptionKeysRotatesTLSConfig(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)

	keyring, err := encrypt.NewKeyring("new-tls-primary-key", "old-tls-retired-key")
	if err != nil {
		t.Fatal(err)
	}
	app.keyring = keyring

	oldKeyring, err := encrypt.NewKeyring("old-tls-retired-key")
	if err != nil {
		t.Fatal(err)
	}

	id := seedLegacyRotationConnection(t, app, "postgres", "postgres://u:p@h:5432/db", oldKeyring)
	blob, err := oldKeyring.Encrypt(`{"mode":"require","server_name":"db.internal","ca_pem":"CA","client_cert_pem":"CERT","client_key_pem":"TLSKEY"}`)
	if err != nil {
		t.Fatal(err)
	}
	setLegacyColumn(t, app, id, "tls_config_encrypted", blob)

	rep, err := app.RotateEncryptionKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ConnectionsSplit != 1 {
		t.Fatalf("report: %+v", rep)
	}

	c, _, _ := app.db.GetConnection(ctx, id)
	if c.TLSConfigEncrypted != "" {
		t.Fatal("legacy tls blob was not cleared")
	}
	var tlsDoc credentials.TLSDocument
	if err := json.Unmarshal(c.TLSConfig, &tlsDoc); err != nil {
		t.Fatalf("decode structured tls config %s: %v", c.TLSConfig, err)
	}
	if tlsDoc.Mode != "require" || tlsDoc.ServerName != "db.internal" || tlsDoc.CAPEM != "CA" || tlsDoc.ClientCertPEM != "CERT" {
		t.Fatalf("structured tls config corrupted by rotation: %+v", tlsDoc)
	}
	if tlsDoc.ClientKeyPEM != "" {
		t.Fatalf("structured tls config retained secret fields: %+v", tlsDoc)
	}
	tlsSecrets, err := app.db.ListConnectionSecrets(ctx, id)
	if err != nil || len(tlsSecrets) != 2 {
		t.Fatalf("split tls secrets = %+v, err=%v", tlsSecrets, err)
	}
	var foundTLSKey bool
	for _, secret := range tlsSecrets {
		if secret.Name != string(credentials.SecretTLSClientKey) {
			continue
		}
		foundTLSKey = true
		plain, err := app.keyring.Decrypt(secret.ValueEncrypted)
		if err != nil || plain != "TLSKEY" {
			t.Fatalf("split tls client key = %q, err=%v", plain, err)
		}
	}
	if !foundTLSKey {
		t.Fatal("split tls client key is missing")
	}

	rep2, err := app.RotateEncryptionKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.ConnectionsSplit != 0 {
		t.Fatalf("second rotation split %d connections; want 0", rep2.ConnectionsSplit)
	}
}

func TestRotateEncryptionKeysRotatesSSHConfig(t *testing.T) {
	ctx := context.Background()
	app := newTestApp(t)

	keyring, err := encrypt.NewKeyring("new-ssh-primary-key", "old-ssh-retired-key")
	if err != nil {
		t.Fatal(err)
	}
	app.keyring = keyring

	oldKeyring, err := encrypt.NewKeyring("old-ssh-retired-key")
	if err != nil {
		t.Fatal(err)
	}

	id := seedLegacyRotationConnection(t, app, "postgres", "postgres://u:p@h:5432/db", oldKeyring)
	blob, err := oldKeyring.Encrypt(`{"enabled":true,"host":"bastion","user":"jump","auth_method":"password","password":"pw","insecure_skip_host_key":true}`)
	if err != nil {
		t.Fatal(err)
	}
	setLegacyColumn(t, app, id, "ssh_config_encrypted", blob)

	rep, err := app.RotateEncryptionKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ConnectionsSplit != 1 {
		t.Fatalf("report: %+v", rep)
	}

	c, _, _ := app.db.GetConnection(ctx, id)
	if c.SSHConfigEncrypted != "" {
		t.Fatal("legacy ssh blob was not cleared")
	}
	var sshDoc credentials.SSHDocument
	if err := json.Unmarshal(c.SSHConfig, &sshDoc); err != nil {
		t.Fatalf("decode structured ssh config %s: %v", c.SSHConfig, err)
	}
	if !sshDoc.Enabled || sshDoc.Host != "bastion" || sshDoc.User != "jump" ||
		sshDoc.AuthMethod != "password" || !sshDoc.InsecureSkipHostKey {
		t.Fatalf("structured ssh config corrupted by rotation: %+v", sshDoc)
	}
	if sshDoc.Password != "" || sshDoc.PrivateKeyPEM != "" || sshDoc.Passphrase != "" {
		t.Fatalf("structured ssh config retained secret fields: %+v", sshDoc)
	}
	secrets, err := app.db.ListConnectionSecrets(ctx, id)
	if err != nil || len(secrets) != 2 {
		t.Fatalf("split secrets = %+v, err=%v", secrets, err)
	}

	rep2, err := app.RotateEncryptionKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep2.ConnectionsSplit != 0 {
		t.Fatalf("second rotation split %d connections; want 0", rep2.ConnectionsSplit)
	}
}

func TestRotateEncryptionKeysIncludesSMTPPassword(t *testing.T) {
	app := newTestApplication(t)
	oldKeyring, err := encrypt.NewKeyring("old-smtp-key")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := oldKeyring.Encrypt("smtp-secret")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := app.instanceSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.SMTPPasswordEncrypted = ciphertext
	if _, err := app.db.UpsertInstanceSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	app.keyring, err = encrypt.NewKeyring("new-smtp-key", "old-smtp-key")
	if err != nil {
		t.Fatal(err)
	}

	report, err := app.RotateEncryptionKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.SMTPPasswordsScanned != 1 || report.SMTPPasswordsRotated != 1 {
		t.Fatalf("unexpected SMTP rotation report: %+v", report)
	}
	rotated, _, err := app.db.GetInstanceSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if app.keyring.NeedsRotation(rotated.SMTPPasswordEncrypted) {
		t.Fatal("SMTP password still needs rotation")
	}
	plaintext, err := app.keyring.Decrypt(rotated.SMTPPasswordEncrypted)
	if err != nil || plaintext != "smtp-secret" {
		t.Fatalf("rotated SMTP password = %q, err=%v", plaintext, err)
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
	if _, ok := res.BodyFields["connections_split"]; !ok {
		t.Errorf("expected connections_split in response, got %v", res.BodyFields)
	}
	if _, ok := res.BodyFields["file_contents_rotated"]; !ok {
		t.Errorf("expected file_contents_rotated in response, got %v", res.BodyFields)
	}
}

func setLegacyColumn(t *testing.T, app *application, id int64, column, value string) {
	t.Helper()
	if _, err := app.db.NewUpdate().Model((*database.Connection)(nil)).
		Set(column+" = ?", value).
		Where("id = ?", id).
		Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}
