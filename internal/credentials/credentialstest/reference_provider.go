package credentialstest

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/credentials"
)

// MemoryRecord configures one connection in ReferenceProvider.
type MemoryRecord struct {
	Credentials credentials.Credentials
	States      map[credentials.SecretName]credentials.SecretState
	Values      map[credentials.SecretName]string
	ResolveErr  error
}

// ReferenceProvider is a small in-memory provider used to prove the provider
// contract does not depend on encrypted database columns.
type ReferenceProvider struct {
	records map[credentials.ConnectionRef]MemoryRecord
}

// NewReferenceProvider returns an in-memory provider. Records and their maps
// are copied so tests cannot mutate provider state accidentally.
func NewReferenceProvider(records map[credentials.ConnectionRef]MemoryRecord) *ReferenceProvider {
	copied := make(map[credentials.ConnectionRef]MemoryRecord, len(records))
	for ref, record := range records {
		record.States = copyStates(record.States)
		record.Values = copyValues(record.Values)
		copied[ref] = record
	}
	return &ReferenceProvider{records: copied}
}

func (p *ReferenceProvider) Resolve(_ context.Context, ref credentials.ConnectionRef) (credentials.Credentials, error) {
	record, ok := p.records[ref]
	if !ok {
		return credentials.Credentials{}, credentials.ErrNotFound
	}
	if record.ResolveErr != nil {
		return credentials.Credentials{}, record.ResolveErr
	}
	return record.Credentials, nil
}

func (p *ReferenceProvider) Describe(_ context.Context, ref credentials.ConnectionRef) (map[credentials.SecretName]credentials.SecretState, error) {
	record, ok := p.records[ref]
	if !ok {
		return nil, credentials.ErrNotFound
	}
	return copyStates(record.States), nil
}

func (p *ReferenceProvider) Reveal(_ context.Context, ref credentials.ConnectionRef, name credentials.SecretName) (string, error) {
	record, ok := p.records[ref]
	if !ok {
		return "", credentials.ErrNotFound
	}
	state := record.States[name]
	if !state.Set || state.Source != credentials.SourceStored {
		return "", credentials.ErrNotRevealable
	}
	value, ok := record.Values[name]
	if !ok {
		return "", credentials.ErrNotRevealable
	}
	return value, nil
}

func copyStates(in map[credentials.SecretName]credentials.SecretState) map[credentials.SecretName]credentials.SecretState {
	out := make(map[credentials.SecretName]credentials.SecretState, len(in))
	for name, state := range in {
		out[name] = state
	}
	return out
}

func copyValues(in map[credentials.SecretName]string) map[credentials.SecretName]string {
	out := make(map[credentials.SecretName]string, len(in))
	for name, value := range in {
		out[name] = value
	}
	return out
}

var _ credentials.Provider = (*ReferenceProvider)(nil)

// ReferenceUnavailableError returns a non-secret failure naming the connection
// for contract fixtures that exercise provider failure handling.
func ReferenceUnavailableError(ref credentials.ConnectionRef) error {
	return fmt.Errorf("reference provider: connection %s unavailable", ref.ConnectionID)
}
