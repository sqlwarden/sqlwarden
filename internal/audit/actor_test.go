package audit

import (
	"testing"

	"github.com/sqlwarden/internal/access"
)

func TestActorFromPrincipal(t *testing.T) {
	p := access.Principal{
		Subject:    access.SubjectRef{Kind: access.SubjectAccount, ID: 4},
		OnBehalfOf: &access.SubjectRef{Kind: access.SubjectTeam, ID: 9},
		Credential: access.CredentialInfo{Kind: access.CredentialSession, ID: "s1", Method: "password", Assurance: access.AAL1, ClientID: "c"},
	}
	a := ActorFromPrincipal(p)
	if a.SubjectKind != "account" || *a.SubjectID != 4 || a.OnBehalfOfKind != "team" || *a.OnBehalfOfID != 9 {
		t.Fatalf("actor = %+v", a)
	}
	if a.CredentialKind != "session" || a.CredentialID != "s1" || a.AuthMethod != "password" || a.Assurance != "aal1" || a.ClientID != "c" {
		t.Fatalf("actor = %+v", a)
	}
}

func TestAnonymousActor(t *testing.T) {
	a := AnonymousActor("password")
	if a.SubjectID != nil || a.SubjectKind != "" || a.AuthMethod != "password" {
		t.Fatalf("actor = %+v", a)
	}
}
