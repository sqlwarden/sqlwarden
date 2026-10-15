package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/uptrace/bun"
)

type ConnectionSecret struct {
	bun.BaseModel `bun:"table:connection_secrets"`

	ConnectionID   int64     `bun:",pk" json:"connection_id"`
	Name           string    `bun:",pk" json:"name"`
	Source         string    `bun:",notnull" json:"source"`
	ValueEncrypted string    `bun:",notnull" json:"-"`
	KeyID          string    `bun:",notnull" json:"-"`
	UpdatedAt      time.Time `bun:",notnull" json:"updated_at"`
}

// LoadConnectionCredentials loads the structured, non-secret portion of a
// connection only when the full organization/workspace hierarchy matches.
func (db *DB) LoadConnectionCredentials(ctx context.Context, orgID, workspaceID, connectionID int64) (
	driver, defaultScope string,
	params, tlsConfig, sshConfig json.RawMessage,
	found bool,
	err error,
) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var conn Connection
	err = db.NewSelect().Model(&conn).
		Join("JOIN workspaces AS w ON w.id = connection.workspace_id").
		Where("connection.id = ?", connectionID).
		Where("connection.workspace_id = ?", workspaceID).
		Where("w.org_id = ?", orgID).
		Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil, nil, nil, false, nil
	}
	if err != nil {
		return "", "", nil, nil, nil, false, err
	}
	return conn.Driver, string(conn.DefaultScope), conn.Params, conn.TLSConfig, conn.SSHConfig, true, nil
}

func (db *DB) ListConnectionSecrets(ctx context.Context, connID int64) ([]ConnectionSecret, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var secrets []ConnectionSecret
	err := db.NewSelect().Model(&secrets).
		Where("connection_id = ?", connID).
		OrderExpr("name ASC").
		Scan(ctx)
	return secrets, err
}

// ListConnectionSecretValues is the primitive projection consumed by the
// credentials Store port.
func (db *DB) ListConnectionSecretValues(ctx context.Context, connID int64) (
	sources, ciphertexts map[string]string,
	err error,
) {
	secrets, err := db.ListConnectionSecrets(ctx, connID)
	if err != nil {
		return nil, nil, err
	}
	sources = make(map[string]string, len(secrets))
	ciphertexts = make(map[string]string, len(secrets))
	for _, secret := range secrets {
		sources[secret.Name] = secret.Source
		ciphertexts[secret.Name] = secret.ValueEncrypted
	}
	return sources, ciphertexts, nil
}

func (db *DB) UpsertConnectionSecret(ctx context.Context, secret ConnectionSecret) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	secret.UpdatedAt = time.Now()
	_, err := db.NewInsert().Model(&secret).
		On("CONFLICT (connection_id, name) DO UPDATE").
		Set("source = EXCLUDED.source").
		Set("value_encrypted = EXCLUDED.value_encrypted").
		Set("key_id = EXCLUDED.key_id").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// UpsertConnectionSecretValue is the primitive projection consumed by the
// credentials Store port.
func (db *DB) UpsertConnectionSecretValue(ctx context.Context, connID int64, name, source, ciphertext, keyID string) error {
	return db.UpsertConnectionSecret(ctx, ConnectionSecret{
		ConnectionID:   connID,
		Name:           name,
		Source:         source,
		ValueEncrypted: ciphertext,
		KeyID:          keyID,
	})
}

func (db *DB) DeleteConnectionSecret(ctx context.Context, connID int64, name string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewDelete().Model((*ConnectionSecret)(nil)).
		Where("connection_id = ?", connID).
		Where("name = ?", name).
		Exec(ctx)
	return err
}
