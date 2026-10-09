package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// RotationStore is the metadata access Rotator needs. Methods use primitive
// types so the metadata database can satisfy it without importing this
// package.
type RotationStore interface {
	ListLegacyConnectionIDs(ctx context.Context) ([]int64, error)
	GetLegacyConnection(ctx context.Context, connectionID int64) (
		driver, dsnEncrypted, tlsEncrypted, sshEncrypted string,
		hasDSN, hasTLS, hasSSH bool,
		found bool,
		err error,
	)
	// SplitLegacyConnection atomically writes the structured columns and
	// secrets for one connection and clears its legacy columns.
	SplitLegacyConnection(
		ctx context.Context,
		connectionID int64,
		consumeDSN, consumeTLS, consumeSSH bool,
		params, tlsConfig, sshConfig json.RawMessage,
		secretNames, secretCiphertexts, secretKeyIDs []string,
	) (split bool, err error)
	ListConnectionIDsWithSecrets(ctx context.Context) ([]int64, error)
	ListConnectionSecretValues(ctx context.Context, connectionID int64) (
		sources, ciphertexts map[string]string,
		err error,
	)
	ReencryptConnectionSecrets(ctx context.Context, connectionID int64, names, ciphertexts, keyIDs []string) error
}

// RotationKeyring is the application keyring Rotator opens and seals with.
type RotationKeyring interface {
	Decrypter
	Encrypter
	NeedsRotation(ciphertext string) bool
	PrimaryKeyID() string
}

// RotationReport counts rotation outcomes. It never carries values.
type RotationReport struct {
	// Split is the number of legacy connections converted to the structured
	// format.
	Split int
	// Reencrypted is the number of stored secrets re-sealed with the primary
	// key.
	Reencrypted int
	// Skipped is the number of rows concurrently split by another runner,
	// stored secrets already sealed with the primary key, and reference-sourced
	// secrets that are not ours to rotate.
	Skipped int
}

// Rotator converts legacy connection rows and re-seals stored secrets with the
// keyring's primary key. Each connection is committed on its own, so a failure
// leaves earlier connections converted and the failing one untouched.
type Rotator struct {
	store   RotationStore
	keyring RotationKeyring
	specs   SpecLookup
}

func NewRotator(store RotationStore, keyring RotationKeyring, specs SpecLookup) *Rotator {
	return &Rotator{store: store, keyring: keyring, specs: specs}
}

// Run splits legacy rows, then re-encrypts stored secrets that are not sealed
// with the primary key. It is idempotent.
func (r *Rotator) Run(ctx context.Context) (RotationReport, error) {
	var report RotationReport

	ids, err := r.store.ListLegacyConnectionIDs(ctx)
	if err != nil {
		return report, fmt.Errorf("credentials: list legacy connections: %w", err)
	}
	for _, id := range ids {
		split, err := r.split(ctx, id)
		if err != nil {
			return report, err
		}
		if split {
			report.Split++
		} else {
			report.Skipped++
		}
	}

	secretIDs, err := r.store.ListConnectionIDsWithSecrets(ctx)
	if err != nil {
		return report, fmt.Errorf("credentials: list connections with secrets: %w", err)
	}
	for _, id := range secretIDs {
		if err := r.reencrypt(ctx, id, &report); err != nil {
			return report, err
		}
	}
	return report, nil
}

func (r *Rotator) split(ctx context.Context, id int64) (bool, error) {
	driver, dsnEnc, tlsEnc, sshEnc, hasDSN, hasTLS, hasSSH, found, err := r.store.GetLegacyConnection(ctx, id)
	if err != nil {
		return false, fmt.Errorf("credentials: load legacy connection %d failed", id)
	}
	if !found {
		return false, nil
	}

	var dsn string
	if hasDSN {
		if dsn, err = r.keyring.Decrypt(dsnEnc); err != nil {
			return false, legacyParseError(id)
		}
	}
	tlsDoc, decodedTLS, err := DecodeTLSDocument(r.keyring, tlsEnc)
	if err != nil {
		return false, legacyParseError(id)
	}
	sshDoc, decodedSSH, err := DecodeSSHDocument(r.keyring, sshEnc)
	if err != nil {
		return false, legacyParseError(id)
	}
	structured, err := splitLegacy(r.specs, driver, dsn, tlsDoc, decodedTLS, sshDoc, decodedSSH)
	if err != nil {
		return false, legacyParseError(id)
	}

	names := make([]string, 0, len(structured.secrets))
	for name := range structured.secrets {
		names = append(names, string(name))
	}
	sort.Strings(names)
	ciphertexts := make([]string, len(names))
	keyIDs := make([]string, len(names))
	for i, name := range names {
		ciphertext, err := r.keyring.Encrypt(structured.secrets[SecretName(name)])
		if err != nil {
			return false, fmt.Errorf("credentials: seal secret %q for connection %d failed", name, id)
		}
		ciphertexts[i] = ciphertext
		keyIDs[i] = r.keyring.PrimaryKeyID()
	}

	split, err := r.store.SplitLegacyConnection(
		ctx, id, hasDSN, hasTLS, hasSSH,
		structured.params, structured.tlsConfig, structured.sshConfig, names, ciphertexts, keyIDs,
	)
	if err != nil {
		return false, fmt.Errorf("credentials: store converted connection %d failed", id)
	}
	return split, nil
}

func (r *Rotator) reencrypt(ctx context.Context, id int64, report *RotationReport) error {
	sources, ciphertexts, err := r.store.ListConnectionSecretValues(ctx, id)
	if err != nil {
		return fmt.Errorf("credentials: load secrets for connection %d failed", id)
	}
	names := make([]string, 0, len(ciphertexts))
	for name := range ciphertexts {
		names = append(names, name)
	}
	sort.Strings(names)
	rotatedNames := make([]string, 0, len(names))
	rotatedCiphertexts := make([]string, 0, len(names))
	rotatedKeyIDs := make([]string, 0, len(names))
	for _, name := range names {
		source := sources[name]
		ciphertext := ciphertexts[name]
		if SecretSource(source) != SourceStored || !r.keyring.NeedsRotation(ciphertext) {
			report.Skipped++
			continue
		}
		plaintext, err := r.keyring.Decrypt(ciphertext)
		if err != nil {
			return fmt.Errorf("credentials: decrypt secret %q for connection %d failed", name, id)
		}
		sealed, err := r.keyring.Encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("credentials: seal secret %q for connection %d failed", name, id)
		}
		rotatedNames = append(rotatedNames, name)
		rotatedCiphertexts = append(rotatedCiphertexts, sealed)
		rotatedKeyIDs = append(rotatedKeyIDs, r.keyring.PrimaryKeyID())
	}
	if len(rotatedNames) == 0 {
		return nil
	}
	if err := r.store.ReencryptConnectionSecrets(ctx, id, rotatedNames, rotatedCiphertexts, rotatedKeyIDs); err != nil {
		return fmt.Errorf("credentials: store re-encrypted secrets for connection %d failed", id)
	}
	report.Reencrypted += len(rotatedNames)
	return nil
}
