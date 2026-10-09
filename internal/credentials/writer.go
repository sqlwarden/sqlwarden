package credentials

import (
	"context"
	"fmt"
)

// Writer changes one stored connection secret at a time.
type Writer interface {
	Set(ctx context.Context, ref ConnectionRef, name SecretName, value string) error
	Clear(ctx context.Context, ref ConnectionRef, name SecretName) error
}

// Set seals and stores one secret as a stored-source value.
func (p *EncryptedColumnProvider) Set(ctx context.Context, ref ConnectionRef, name SecretName, value string) error {
	if !knownSecretName(name) {
		return ErrUnknownSecret
	}
	conn, err := p.load(ctx, ref)
	if err != nil {
		return err
	}
	ciphertext, keyID, err := p.sealer.Seal(value)
	if err != nil {
		return fmt.Errorf("credentials: seal secret %q for connection %d failed", name, conn.id)
	}
	if err := p.store.UpsertConnectionSecretValue(
		ctx, conn.id, string(name), string(SourceStored), ciphertext, keyID,
	); err != nil {
		return fmt.Errorf("credentials: store secret %q for connection %d failed", name, conn.id)
	}
	return nil
}

// Clear removes one stored secret.
func (p *EncryptedColumnProvider) Clear(ctx context.Context, ref ConnectionRef, name SecretName) error {
	if !knownSecretName(name) {
		return ErrUnknownSecret
	}
	conn, err := p.load(ctx, ref)
	if err != nil {
		return err
	}
	if err := p.store.DeleteConnectionSecret(ctx, conn.id, string(name)); err != nil {
		return fmt.Errorf("credentials: clear secret %q for connection %d failed", name, conn.id)
	}
	return nil
}

var _ Writer = (*EncryptedColumnProvider)(nil)
