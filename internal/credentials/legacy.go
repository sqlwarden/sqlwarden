package credentials

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/engine"
)

// Store is the metadata access LegacyDSNProvider needs.
type Store interface {
	GetConnection(ctx context.Context, id int64) (database.Connection, bool, error)
	GetWorkspace(ctx context.Context, id int64) (database.Workspace, bool, error)
}

// Decrypter opens values sealed by the application keyring.
type Decrypter interface {
	Decrypt(ciphertext string) (string, error)
}

// LegacyDSNProvider reads the encrypted DSN and SSH columns on the connection
// row.
type LegacyDSNProvider struct {
	store   Store
	keyring Decrypter
}

func NewLegacyDSNProvider(store Store, keyring Decrypter) *LegacyDSNProvider {
	return &LegacyDSNProvider{store: store, keyring: keyring}
}

func (p *LegacyDSNProvider) Resolve(ctx context.Context, ref ConnectionRef) (Credentials, error) {
	// Malformed IDs collapse to ErrNotFound so a caller cannot distinguish a
	// bad ref from a missing or foreign connection.
	orgID, err1 := strconv.ParseInt(ref.OrgID, 10, 64)
	wsID, err2 := strconv.ParseInt(ref.WorkspaceID, 10, 64)
	connID, err3 := strconv.ParseInt(ref.ConnectionID, 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return Credentials{}, ErrNotFound
	}

	conn, ok, err := p.store.GetConnection(ctx, connID)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: load connection: %w", err)
	}
	if !ok || conn.WorkspaceID != wsID {
		return Credentials{}, ErrNotFound
	}
	ws, ok, err := p.store.GetWorkspace(ctx, wsID)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: load workspace: %w", err)
	}
	if !ok || ws.OrgID == nil || *ws.OrgID != orgID {
		return Credentials{}, ErrNotFound
	}

	// Decrypter errors are not wrapped: an implementation could echo the
	// value it failed on.
	dsn, err := p.keyring.Decrypt(conn.DSNEncrypted)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: decrypt dsn for connection %d failed", connID)
	}
	doc, has, err := DecodeSSHDocument(p.keyring, conn.SSHConfigEncrypted)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: connection %d: %w", connID, err)
	}
	var ssh *SSHConfig
	if has {
		ssh = doc.ToConfig()
	}
	tlsDoc, hasTLS, err := DecodeTLSDocument(p.keyring, conn.TLSConfigEncrypted)
	if err != nil {
		return Credentials{}, fmt.Errorf("credentials: connection %d: %w", connID, err)
	}
	var tlsCfg *engine.TLSConfig
	if hasTLS {
		tlsCfg = tlsDoc.ToEngine()
	}

	return Credentials{
		Driver:       conn.Driver,
		DefaultScope: string(conn.DefaultScope),
		DSN:          dsn,
		SSH:          ssh,
		TLS:          tlsCfg,
	}, nil
}
