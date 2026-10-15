package database

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/encrypt"
	"github.com/sqlwarden/internal/engine"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
)

func TestConnectionSecretsStore(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-secrets")

			if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
				ConnectionID:   conn.ID,
				Name:           "password",
				Source:         "stored",
				ValueEncrypted: "ciphertext-1",
				KeyID:          "key-1",
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
				ConnectionID:   conn.ID,
				Name:           "ssh_password",
				Source:         "reference",
				ValueEncrypted: "vault/path",
				KeyID:          "",
			}); err != nil {
				t.Fatal(err)
			}

			secrets, err := db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(secrets) != 2 || secrets[0].Name != "password" || secrets[1].Name != "ssh_password" {
				t.Fatalf("unexpected secrets: %+v", secrets)
			}
			if secrets[0].UpdatedAt.IsZero() {
				t.Fatal("expected updated_at to be set")
			}

			if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
				ConnectionID:   conn.ID,
				Name:           "password",
				Source:         "stored",
				ValueEncrypted: "ciphertext-2",
				KeyID:          "key-2",
			}); err != nil {
				t.Fatal(err)
			}
			secrets, err = db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(secrets) != 2 || secrets[0].ValueEncrypted != "ciphertext-2" || secrets[0].KeyID != "key-2" {
				t.Fatalf("upsert did not replace the secret: %+v", secrets)
			}

			if err := db.DeleteConnectionSecret(ctx, conn.ID, "ssh_password"); err != nil {
				t.Fatal(err)
			}
			secrets, err = db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(secrets) != 1 || secrets[0].Name != "password" {
				t.Fatalf("unexpected secrets after delete: %+v", secrets)
			}
		})
	}
}

func TestConnectionSecretCascadeDelete(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-cascade")
			if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
				ConnectionID:   conn.ID,
				Name:           "password",
				Source:         "stored",
				ValueEncrypted: "ciphertext",
				KeyID:          "key-1",
			}); err != nil {
				t.Fatal(err)
			}

			if err := db.DeleteConnection(ctx, conn.ID); err != nil {
				t.Fatal(err)
			}
			secrets, err := db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(secrets) != 0 {
				t.Fatalf("expected cascade delete, got %+v", secrets)
			}
		})
	}
}

func TestReencryptConnectionSecretsIsAtomic(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-reencrypt-atomic")
			for _, name := range []string{"password", "ssh_password"} {
				if err := db.UpsertConnectionSecret(ctx, ConnectionSecret{
					ConnectionID: conn.ID, Name: name, Source: "stored", ValueEncrypted: "old-" + name, KeyID: "old-key",
				}); err != nil {
					t.Fatal(err)
				}
			}

			err := db.ReencryptConnectionSecrets(ctx, conn.ID,
				[]string{"password", "missing"}, []string{"new-password", "new-missing"}, []string{"new-key", "new-key"})
			if err == nil {
				t.Fatal("expected missing secret to abort the transaction")
			}
			secrets, err := db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			if secrets[0].ValueEncrypted != "old-password" || secrets[0].KeyID != "old-key" {
				t.Fatalf("first secret changed despite rollback: %+v", secrets[0])
			}

			if err := db.ReencryptConnectionSecrets(ctx, conn.ID,
				[]string{"password", "ssh_password"}, []string{"new-password", "new-ssh"}, []string{"new-key", "new-key"}); err != nil {
				t.Fatal(err)
			}
			secrets, err = db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range secrets {
				if secret.KeyID != "new-key" || secret.ValueEncrypted[:4] != "new-" {
					t.Fatalf("secret was not re-encrypted: %+v", secret)
				}
			}
		})
	}
}

func TestSplitLegacyConnectionIsAtomic(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-split-atomic")
			oldParams := json.RawMessage(`{"host":"old.example.com"}`)
			oldTLS := json.RawMessage(`{"mode":"verify-ca"}`)
			oldSSH := json.RawMessage(`{"enabled":true,"host":"existing-bastion"}`)
			if err := db.UpdateConnectionStructured(ctx, conn.ID, oldParams, oldTLS, oldSSH); err != nil {
				t.Fatal(err)
			}
			newParams := json.RawMessage(`{"host":"db.example.com"}`)

			split, err := db.SplitLegacyConnection(ctx, conn.ID, true, false, false, newParams, nil, nil,
				[]string{""}, []string{"ciphertext"}, []string{"active-key"})
			if err == nil {
				t.Fatal("expected invalid secret row to fail after the connection update")
			}
			if split {
				t.Fatal("failed transaction reported the connection as split")
			}
			stored, found, err := db.GetConnection(ctx, conn.ID)
			if err != nil || !found {
				t.Fatalf("GetConnection: found=%v err=%v", found, err)
			}
			if stored.DSNEncrypted != "legacy-ciphertext" {
				t.Fatal("legacy row changed despite transaction rollback")
			}
			assertJSONEqual(t, stored.Params, oldParams)
			assertJSONEqual(t, stored.TLSConfig, oldTLS)
			assertJSONEqual(t, stored.SSHConfig, oldSSH)
			secrets, err := db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil || len(secrets) != 0 {
				t.Fatalf("secrets after rolled-back split = %+v, err=%v", secrets, err)
			}

			split, err = db.SplitLegacyConnection(ctx, conn.ID, true, false, false, newParams, nil, nil,
				[]string{"password"}, []string{"ciphertext"}, []string{"active-key"})
			if err != nil {
				t.Fatal(err)
			}
			if !split {
				t.Fatal("legacy connection was not split")
			}
			stored, found, err = db.GetConnection(ctx, conn.ID)
			if err != nil || !found {
				t.Fatalf("GetConnection: found=%v err=%v", found, err)
			}
			if stored.DSNEncrypted != "" || stored.TLSConfigEncrypted != "" || stored.SSHConfigEncrypted != "" {
				t.Fatalf("legacy columns were not cleared: %+v", stored)
			}
			assertJSONEqual(t, stored.Params, newParams)
			assertJSONEqual(t, stored.TLSConfig, oldTLS)
			assertJSONEqual(t, stored.SSHConfig, oldSSH)
			secrets, err = db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil || len(secrets) != 1 || secrets[0].KeyID != "active-key" {
				t.Fatalf("split secrets = %+v, err=%v", secrets, err)
			}

			split, err = db.SplitLegacyConnection(ctx, conn.ID, true, false, false, json.RawMessage(`{"host":"wrong.example.com"}`), nil, nil,
				[]string{"ssh_password"}, []string{"other"}, []string{"active-key"})
			if err != nil || split {
				t.Fatalf("already-split result: split=%v err=%v", split, err)
			}
			stored, _, _ = db.GetConnection(ctx, conn.ID)
			assertJSONEqual(t, stored.Params, newParams)
			secrets, err = db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil || len(secrets) != 1 {
				t.Fatalf("already-split call wrote secrets: %+v, err=%v", secrets, err)
			}

			partial := insertConnectionSecretTestConnection(t, db, driver+"-split-partial")
			if err := db.UpdateConnectionStructured(ctx, partial.ID, oldParams, oldTLS, oldSSH); err != nil {
				t.Fatal(err)
			}
			if _, err := db.NewUpdate().Model((*Connection)(nil)).
				Set("dsn_encrypted = NULL").
				Set("tls_config_encrypted = ?", "legacy-tls").
				Where("id = ?", partial.ID).
				Exec(ctx); err != nil {
				t.Fatal(err)
			}
			newTLS := json.RawMessage(`{"mode":"require","server_name":"new.example.com"}`)
			split, err = db.SplitLegacyConnection(ctx, partial.ID, false, true, false,
				json.RawMessage(`{"host":"must-not-overwrite.example.com"}`), newTLS,
				json.RawMessage(`{"enabled":false}`), nil, nil, nil)
			if err != nil || !split {
				t.Fatalf("partial split: split=%v err=%v", split, err)
			}
			stored, found, err = db.GetConnection(ctx, partial.ID)
			if err != nil || !found {
				t.Fatalf("GetConnection: found=%v err=%v", found, err)
			}
			assertJSONEqual(t, stored.Params, oldParams)
			assertJSONEqual(t, stored.TLSConfig, newTLS)
			assertJSONEqual(t, stored.SSHConfig, oldSSH)
			if stored.TLSConfigEncrypted != "" {
				t.Fatal("consumed TLS legacy column was not cleared")
			}
		})
	}
}

func TestCredentialsRotatorWithDatabaseStore(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-rotator")

			oldKeyring, err := encrypt.NewKeyring("old-rotation-key")
			if err != nil {
				t.Fatal(err)
			}
			activeKeyring, err := encrypt.NewKeyring("active-rotation-key", "old-rotation-key")
			if err != nil {
				t.Fatal(err)
			}
			legacyDSN := "postgresql://app:legacy-password@db.example.com:5432/app?sslmode=verify-full"
			ciphertext, err := oldKeyring.Encrypt(legacyDSN)
			if err != nil {
				t.Fatal(err)
			}
			setLegacyColumn(t, db, conn.ID, "dsn_encrypted", ciphertext)

			report, err := credentials.NewRotator(db, activeKeyring, engine.ConnectionSpecFor).Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if report.Split != 1 || report.Reencrypted != 0 {
				t.Fatalf("rotation report = %+v", report)
			}
			stored, found, err := db.GetConnection(ctx, conn.ID)
			if err != nil || !found {
				t.Fatalf("GetConnection: found=%v err=%v", found, err)
			}
			if stored.DSNEncrypted != "" {
				t.Fatal("legacy DSN was not cleared")
			}
			var params map[string]string
			if err := json.Unmarshal(stored.Params, &params); err != nil || params["host"] != "db.example.com" {
				t.Fatalf("structured params = %s, err=%v", stored.Params, err)
			}
			var tlsConfig map[string]string
			if err := json.Unmarshal(stored.TLSConfig, &tlsConfig); err != nil || tlsConfig["mode"] != "verify-full" {
				t.Fatalf("structured tls config = %s, err=%v", stored.TLSConfig, err)
			}
			secrets, err := db.ListConnectionSecrets(ctx, conn.ID)
			if err != nil || len(secrets) != 1 || secrets[0].Name != "password" {
				t.Fatalf("split secrets = %+v, err=%v", secrets, err)
			}
			plain, err := activeKeyring.Decrypt(secrets[0].ValueEncrypted)
			if err != nil || plain != "legacy-password" || activeKeyring.NeedsRotation(secrets[0].ValueEncrypted) {
				t.Fatalf("split password = %q, err=%v", plain, err)
			}

			again, err := credentials.NewRotator(db, activeKeyring, engine.ConnectionSpecFor).Run(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if again.Split != 0 || again.Reencrypted != 0 || again.Skipped != 1 {
				t.Fatalf("idempotent rotation report = %+v", again)
			}
		})
	}
}

func TestConnectionStructuredFieldsAndLegacyCount(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-structured")

			count, err := db.CountLegacyConnections(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("legacy count = %d, want 1", count)
			}

			params := json.RawMessage(`{"host":"localhost","port":"5432"}`)
			tlsConfig := json.RawMessage(`{"mode":"require"}`)
			sshConfig := json.RawMessage(`{"enabled":false}`)
			if err := db.UpdateConnectionStructured(ctx, conn.ID, params, tlsConfig, sshConfig); err != nil {
				t.Fatal(err)
			}
			if _, err := db.NewUpdate().Model((*Connection)(nil)).
				Set("dsn_encrypted = NULL").
				Set("tls_config_encrypted = NULL").
				Set("ssh_config_encrypted = NULL").
				Where("id = ?", conn.ID).
				Exec(ctx); err != nil {
				t.Fatal(err)
			}

			stored, found, err := db.GetConnection(ctx, conn.ID)
			if err != nil || !found {
				t.Fatalf("GetConnection: found=%v err=%v", found, err)
			}
			assertJSONEqual(t, stored.Params, params)
			assertJSONEqual(t, stored.TLSConfig, tlsConfig)
			assertJSONEqual(t, stored.SSHConfig, sshConfig)

			count, err = db.CountLegacyConnections(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("legacy count = %d, want 0", count)
			}
		})
	}
}

func TestEmptyLegacyColumnsAreNotLegacy(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-empty-legacy")
			for _, column := range []string{"dsn_encrypted", "tls_config_encrypted", "ssh_config_encrypted"} {
				setLegacyColumn(t, db, conn.ID, column, "")
			}

			count, err := db.CountLegacyConnections(ctx)
			if err != nil || count != 0 {
				t.Fatalf("legacy count = %d, err=%v, want 0", count, err)
			}
			ids, err := db.ListLegacyConnectionIDs(ctx)
			if err != nil || len(ids) != 0 {
				t.Fatalf("legacy ids = %v, err=%v, want none", ids, err)
			}
			_, _, _, _, hasDSN, hasTLS, hasSSH, found, err := db.GetLegacyConnection(ctx, conn.ID)
			if err != nil || !found || hasDSN || hasTLS || hasSSH {
				t.Fatalf("GetLegacyConnection flags = %v %v %v found=%v err=%v", hasDSN, hasTLS, hasSSH, found, err)
			}
		})
	}
}

func TestUpdateConnectionLeavesLegacyColumnsUntouched(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-update-legacy")
			if _, err := db.NewUpdate().Model((*Connection)(nil)).
				Set("dsn_encrypted = NULL").Where("id = ?", conn.ID).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.UpdateConnectionWithScopeAndPolicy(ctx, conn.ID, "renamed", "open", SchemaSnapshotPolicyInherit, conn.DefaultScope, false, false); err != nil {
				t.Fatal(err)
			}
			count, err := db.CountLegacyConnections(ctx)
			if err != nil || count != 0 {
				t.Fatalf("legacy count after update = %d, err=%v, want 0", count, err)
			}
		})
	}
}

func setLegacyColumn(t *testing.T, db *DB, id int64, column, value string) {
	t.Helper()
	if _, err := db.NewUpdate().Model((*Connection)(nil)).
		Set(column+" = ?", value).
		Where("id = ?", id).
		Exec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func assertJSONEqual(t *testing.T, got, want json.RawMessage) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("invalid stored JSON %q: %v", got, err)
	}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("invalid expected JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("stored JSON = %s, want %s", got, want)
	}
}

func TestOrganizationConnectionSecretRevealDefaultsFalse(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, driver+"-reveal-default", "Reveal Default")
			if err != nil {
				t.Fatal(err)
			}
			stored, found, err := db.GetOrg(ctx, org.ID)
			if err != nil || !found {
				t.Fatalf("GetOrg: found=%v err=%v", found, err)
			}
			if stored.AllowConnectionSecretReveal {
				t.Fatal("allow_connection_secret_reveal must default to false")
			}
		})
	}
}

func TestOrganizationConnectionSecretRevealCanBeToggled(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, driver+"-reveal-toggle", "Reveal Toggle")
			if err != nil {
				t.Fatal(err)
			}

			for _, allowed := range []bool{true, false} {
				if err := db.UpdateOrgSettings(ctx, org.ID, nil, nil, &allowed); err != nil {
					t.Fatal(err)
				}
				stored, found, err := db.GetOrg(ctx, org.ID)
				if err != nil || !found {
					t.Fatalf("GetOrg: found=%v err=%v", found, err)
				}
				if stored.AllowConnectionSecretReveal != allowed {
					t.Fatalf("allow_connection_secret_reveal = %v, want %v", stored.AllowConnectionSecretReveal, allowed)
				}
			}
		})
	}
}

func TestConnectionParamsDefaultsForLegacyInsert(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			org, err := db.InsertOrg(ctx, driver+"-params-default", "Params Default")
			if err != nil {
				t.Fatal(err)
			}
			ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
			if err != nil {
				t.Fatal(err)
			}
			envID, err := db.defaultEnvironmentIDWithExecutor(ctx, db.DB, ws.ID)
			if err != nil {
				t.Fatal(err)
			}

			row := map[string]any{
				"workspace_id":   ws.ID,
				"environment_id": envID,
				"name":           "legacy",
				"driver":         "postgres",
				"dsn_encrypted":  "legacy-ciphertext",
				"access_mode":    "open",
				"created_at":     time.Now(),
				"updated_at":     time.Now(),
			}
			var connectionID int64
			if err := db.NewInsert().TableExpr("connections").Model(&row).Returning("id").Scan(ctx, &connectionID); err != nil {
				t.Fatal(err)
			}
			stored, found, err := db.GetConnection(ctx, connectionID)
			if err != nil || !found {
				t.Fatalf("GetConnection: found=%v err=%v", found, err)
			}
			if string(stored.Params) != `{}` {
				t.Fatalf("params = %s, want {}", stored.Params)
			}
		})
	}
}

func TestListLegacyConnectionIDsIncludesPersonalSpaces(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()

			orgConn := insertConnectionSecretTestConnection(t, db, driver+"-list-all-org")
			account, err := db.InsertAccount(ctx, driver+"-personal-list@example.com", "Personal List", nil)
			if err != nil {
				t.Fatal(err)
			}
			personalWorkspace, err := db.InsertWorkspace(ctx, nil, "user", account.ID, "Personal", "")
			if err != nil {
				t.Fatal(err)
			}
			personalConn, err := db.InsertConnection(ctx, personalWorkspace.ID, nil, "personal", "sqlite", "legacy-personal", "open")
			if err != nil {
				t.Fatal(err)
			}

			legacyIDs, err := db.ListLegacyConnectionIDs(ctx)
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[int64]bool, len(legacyIDs))
			for _, id := range legacyIDs {
				seen[id] = true
			}
			if !seen[orgConn.ID] || !seen[personalConn.ID] {
				t.Fatalf("ListLegacyConnectionIDs omitted org or personal connection: %+v", legacyIDs)
			}
		})
	}
}

func insertConnectionSecretTestConnection(t *testing.T, db *DB, suffix string) Connection {
	t.Helper()
	ctx := context.Background()
	org, err := db.InsertOrg(ctx, fmt.Sprintf("connection-secret-%s", suffix), "Connection Secret Test")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := db.InsertWorkspace(ctx, &org.ID, "org", org.ID, "Main", "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.InsertConnection(ctx, ws.ID, nil, "primary", "postgres", "legacy-ciphertext", "open")
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestLoadConnectionCredentialsScoping(t *testing.T) {
	for _, driver := range testDrivers() {
		t.Run(driver, func(t *testing.T) {
			db := newTestDB(t, driver)
			ctx := context.Background()
			conn := insertConnectionSecretTestConnection(t, db, driver+"-load")
			ws, ok, err := db.GetWorkspace(ctx, conn.WorkspaceID)
			if err != nil || !ok || ws.OrgID == nil {
				t.Fatalf("workspace lookup: ok=%v err=%v", ok, err)
			}
			orgID := *ws.OrgID

			gotDriver, _, params, _, _, found, err := db.LoadConnectionCredentials(ctx, orgID, ws.ID, conn.ID)
			if err != nil || !found || gotDriver != "postgres" {
				t.Fatalf("matching ids: driver=%q found=%v err=%v", gotDriver, found, err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(params, &decoded); err != nil {
				t.Fatalf("params not JSON: %v", err)
			}

			otherOrg, err := db.InsertOrg(ctx, driver+"-load-other", "Other")
			if err != nil {
				t.Fatal(err)
			}
			otherWS, err := db.InsertWorkspace(ctx, &otherOrg.ID, "org", otherOrg.ID, "Other", "")
			if err != nil {
				t.Fatal(err)
			}
			account, err := db.InsertAccount(ctx, driver+"-load-personal@example.com", "Personal Load", nil)
			if err != nil {
				t.Fatal(err)
			}
			personalWS, err := db.InsertWorkspace(ctx, nil, "user", account.ID, "Personal", "")
			if err != nil {
				t.Fatal(err)
			}
			personalConn, err := db.InsertConnection(ctx, personalWS.ID, nil, "personal", "postgres", "x", "open")
			if err != nil {
				t.Fatal(err)
			}

			for name, ids := range map[string][3]int64{
				"wrong org":         {otherOrg.ID, ws.ID, conn.ID},
				"wrong workspace":   {orgID, otherWS.ID, conn.ID},
				"unknown":           {orgID, ws.ID, conn.ID + 1000},
				"nil org workspace": {orgID, personalWS.ID, personalConn.ID},
			} {
				_, _, _, _, _, found, err := db.LoadConnectionCredentials(ctx, ids[0], ids[1], ids[2])
				if err != nil || found {
					t.Fatalf("%s: found=%v err=%v", name, found, err)
				}
			}
		})
	}
}
