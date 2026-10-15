package audit

import "github.com/sqlwarden/internal/access"

// ActorFromPrincipal copies the principal fields audit records.
func ActorFromPrincipal(p access.Principal) Actor {
	id := p.Subject.ID
	a := Actor{
		SubjectKind:    string(p.Subject.Kind),
		SubjectID:      &id,
		CredentialKind: string(p.Credential.Kind),
		CredentialID:   p.Credential.ID,
		ClientID:       p.Credential.ClientID,
		AuthMethod:     p.Credential.Method,
		Assurance:      string(p.Credential.Assurance),
	}
	if p.OnBehalfOf != nil {
		obo := p.OnBehalfOf.ID
		a.OnBehalfOfKind = string(p.OnBehalfOf.Kind)
		a.OnBehalfOfID = &obo
	}
	return a
}

// AnonymousActor records an attempt that did not establish a subject.
func AnonymousActor(method string) Actor {
	return Actor{AuthMethod: method}
}
