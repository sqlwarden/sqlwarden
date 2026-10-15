package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
)

// legacyConnectionWhere treats an empty string as absent: only a non-empty
// ciphertext is legacy data that still needs conversion.
const legacyConnectionWhere = "NULLIF(dsn_encrypted, '') IS NOT NULL OR NULLIF(tls_config_encrypted, '') IS NOT NULL OR NULLIF(ssh_config_encrypted, '') IS NOT NULL"

// StructuredConnectionSchemaPresent reports whether the structured connection
// migration has been applied.
func (db *DB) StructuredConnectionSchemaPresent(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var count int
	var err error
	switch db.Dialect().Name() {
	case dialect.PG:
		err = db.NewSelect().
			TableExpr("information_schema.tables").
			ColumnExpr("COUNT(*)").
			Where("table_schema = current_schema()").
			Where("table_name = ?", "connection_secrets").
			Scan(ctx, &count)
	default:
		err = db.NewSelect().
			TableExpr("sqlite_master").
			ColumnExpr("COUNT(*)").
			Where("type = ?", "table").
			Where("name = ?", "connection_secrets").
			Scan(ctx, &count)
	}
	return count > 0, err
}

// ListLegacyConnectionIDs returns connections that still carry legacy
// encrypted columns, in id order.
func (db *DB) ListLegacyConnectionIDs(ctx context.Context) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []int64
	err := db.NewSelect().Model((*Connection)(nil)).
		Column("id").
		Where(legacyConnectionWhere).
		OrderExpr("id ASC").
		Scan(ctx, &ids)
	return ids, err
}

// GetLegacyConnection returns the driver and the legacy encrypted columns of
// one connection. Absent columns come back empty.
func (db *DB) GetLegacyConnection(ctx context.Context, connectionID int64) (
	driver, dsnEncrypted, tlsEncrypted, sshEncrypted string,
	hasDSN, hasTLS, hasSSH bool,
	found bool,
	err error,
) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var row struct {
		Driver             string
		DSNEncrypted       *string
		TLSConfigEncrypted *string
		SSHConfigEncrypted *string
	}
	err = db.NewSelect().Model((*Connection)(nil)).
		Column("driver", "dsn_encrypted", "tls_config_encrypted", "ssh_config_encrypted").
		Where("id = ?", connectionID).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", "", false, false, false, false, nil
	}
	if err != nil {
		return "", "", "", "", false, false, false, false, err
	}
	if row.DSNEncrypted != nil {
		dsnEncrypted = *row.DSNEncrypted
	}
	if row.TLSConfigEncrypted != nil {
		tlsEncrypted = *row.TLSConfigEncrypted
	}
	if row.SSHConfigEncrypted != nil {
		sshEncrypted = *row.SSHConfigEncrypted
	}
	return row.Driver, dsnEncrypted, tlsEncrypted, sshEncrypted,
		dsnEncrypted != "", tlsEncrypted != "", sshEncrypted != "", true, nil
}

// SplitLegacyConnection writes the structured columns and secrets of one
// connection and clears its legacy columns in a single transaction. It leaves
// updated_at untouched so conversion is invisible to consumers.
func (db *DB) SplitLegacyConnection(
	ctx context.Context,
	connectionID int64,
	consumeDSN, consumeTLS, consumeSSH bool,
	params, tlsConfig, sshConfig json.RawMessage,
	secretNames, secretCiphertexts, secretKeyIDs []string,
) (bool, error) {
	if len(secretNames) != len(secretCiphertexts) || len(secretNames) != len(secretKeyIDs) {
		return false, fmt.Errorf("split connection %d: mismatched secret lists", connectionID)
	}
	if !consumeDSN && !consumeTLS && !consumeSSH {
		return false, nil
	}
	legacyGuards := make([]string, 0, 3)
	if consumeDSN {
		legacyGuards = append(legacyGuards, "NULLIF(dsn_encrypted, '') IS NOT NULL")
	}
	if consumeTLS {
		legacyGuards = append(legacyGuards, "NULLIF(tls_config_encrypted, '') IS NOT NULL")
	}
	if consumeSSH {
		legacyGuards = append(legacyGuards, "NULLIF(ssh_config_encrypted, '') IS NOT NULL")
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var split bool
	err := db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		query := tx.NewUpdate().Model((*Connection)(nil))
		if consumeDSN && len(params) > 0 {
			query = query.Set("params = ?", rawJSONValue(params))
		}
		if (consumeDSN || consumeTLS) && len(tlsConfig) > 0 {
			query = query.Set("tls_config = ?", rawJSONValue(tlsConfig))
		}
		if consumeSSH && len(sshConfig) > 0 {
			query = query.Set("ssh_config = ?", rawJSONValue(sshConfig))
		}
		if consumeDSN {
			query = query.Set("dsn_encrypted = NULL")
		}
		if consumeTLS {
			query = query.Set("tls_config_encrypted = NULL")
		}
		if consumeSSH {
			query = query.Set("ssh_config_encrypted = NULL")
		}
		res, err := query.Where("id = ?", connectionID).
			Where("(" + strings.Join(legacyGuards, " OR ") + ")").
			Exec(ctx)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		split = true
		now := time.Now()
		for i, name := range secretNames {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("split connection %d: secret name is empty", connectionID)
			}
			secret := ConnectionSecret{
				ConnectionID:   connectionID,
				Name:           name,
				Source:         "stored",
				ValueEncrypted: secretCiphertexts[i],
				KeyID:          secretKeyIDs[i],
				UpdatedAt:      now,
			}
			if _, err := tx.NewInsert().Model(&secret).
				On("CONFLICT (connection_id, name) DO UPDATE").
				Set("source = EXCLUDED.source").
				Set("value_encrypted = EXCLUDED.value_encrypted").
				Set("key_id = EXCLUDED.key_id").
				Set("updated_at = EXCLUDED.updated_at").
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return split, nil
}

// ListConnectionIDsWithSecrets returns the distinct connections that hold at
// least one stored secret row.
func (db *DB) ListConnectionIDsWithSecrets(ctx context.Context) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var ids []int64
	err := db.NewSelect().Model((*ConnectionSecret)(nil)).
		ColumnExpr("DISTINCT connection_id").
		OrderExpr("connection_id ASC").
		Scan(ctx, &ids)
	return ids, err
}

// ReencryptConnectionSecrets replaces ciphertext and key inventory for one
// connection atomically.
func (db *DB) ReencryptConnectionSecrets(ctx context.Context, connectionID int64, names, ciphertexts, keyIDs []string) error {
	if len(names) != len(ciphertexts) || len(names) != len(keyIDs) {
		return fmt.Errorf("re-encrypt connection %d: mismatched secret lists", connectionID)
	}
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for i, name := range names {
			res, err := tx.NewUpdate().Model((*ConnectionSecret)(nil)).
				Set("value_encrypted = ?", ciphertexts[i]).
				Set("key_id = ?", keyIDs[i]).
				Set("updated_at = ?", time.Now()).
				Where("connection_id = ?", connectionID).
				Where("name = ?", name).
				Where("source = ?", "stored").
				Exec(ctx)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err == nil && n != 1 {
				return sql.ErrNoRows
			}
		}
		return nil
	})
}
