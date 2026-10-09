package credentialstest_test

import (
	"testing"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/credentials/credentialstest"
)

func TestReferenceProviderContract(t *testing.T) {
	ref := credentials.ConnectionRef{OrgID: "1", WorkspaceID: "2", ConnectionID: "3"}
	withSSH := credentials.ConnectionRef{OrgID: "1", WorkspaceID: "2", ConnectionID: "4"}
	bad := credentials.ConnectionRef{OrgID: "1", WorkspaceID: "2", ConnectionID: "5"}
	reference := credentials.ConnectionRef{OrgID: "1", WorkspaceID: "2", ConnectionID: "6"}
	password := "memory-stored-password-do-not-leak"
	referenceValue := "vault/connection/password"
	wantDSN := "memory://user:" + password + "@host/database"

	provider := credentialstest.NewReferenceProvider(map[credentials.ConnectionRef]credentialstest.MemoryRecord{
		ref: {
			Credentials: credentials.Credentials{Driver: "memory", DSN: wantDSN},
			States: map[credentials.SecretName]credentials.SecretState{
				credentials.SecretPassword: {Set: true, Source: credentials.SourceStored},
			},
			Values: map[credentials.SecretName]string{credentials.SecretPassword: password},
		},
		withSSH: {
			Credentials: credentials.Credentials{Driver: "memory", DSN: wantDSN},
			States: map[credentials.SecretName]credentials.SecretState{
				credentials.SecretPassword: {Set: true, Source: credentials.SourceStored},
			},
			Values: map[credentials.SecretName]string{credentials.SecretPassword: password},
		},
		bad: {ResolveErr: credentialstest.ReferenceUnavailableError(bad)},
		reference: {
			Credentials: credentials.Credentials{Driver: "memory", DSN: "memory://reference@host/database"},
			States: map[credentials.SecretName]credentials.SecretState{
				credentials.SecretPassword: {Set: true, Source: credentials.SourceReference},
			},
			Values: map[credentials.SecretName]string{credentials.SecretPassword: referenceValue},
		},
	})

	credentialstest.RunProviderContract(t, provider, credentialstest.Fixture{
		Ref:                  ref,
		WantDSN:              wantDSN,
		WithSSH:              withSSH,
		WrongWorkspace:       credentials.ConnectionRef{OrgID: "1", WorkspaceID: "99", ConnectionID: "3"},
		WrongOrg:             credentials.ConnectionRef{OrgID: "99", WorkspaceID: "2", ConnectionID: "3"},
		Missing:              credentials.ConnectionRef{OrgID: "1", WorkspaceID: "2", ConnectionID: "99"},
		Undecryptable:        bad,
		UndecryptableMention: "connection 5",
		Reference:            reference,
		ReferenceName:        credentials.SecretPassword,
		StoredName:           credentials.SecretPassword,
		StoredValue:          password,
		WantStates: map[credentials.SecretName]credentials.SecretState{
			credentials.SecretPassword: {Set: true, Source: credentials.SourceStored},
		},
		Secrets: []string{password, referenceValue},
	})
}
