package web

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/connection"
	"github.com/sqlwarden/internal/validator"
)

func TestSSHDocumentSealDecodeRoundTrip(t *testing.T) {
	app := newTestApplication(t)
	doc := sshConfigDocument{
		Enabled: true, Host: "bastion.example", Port: 22, User: "jump",
		AuthMethod: string(connection.SSHAuthPassword), Password: "pw",
		Fingerprint: "SHA256:abc",
	}
	sealed, err := app.sealSSHDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "" || strings.Contains(sealed, "pw") {
		t.Fatalf("sealed blob leaks plaintext or is empty: %q", sealed)
	}
	got, has, err := app.decodeSSHDocument(sealed)
	if err != nil || !has {
		t.Fatalf("decode: has=%v err=%v", has, err)
	}
	if got.Password != "pw" || got.Host != "bastion.example" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestSSHDocumentSealEmptyIsBlank(t *testing.T) {
	app := newTestApplication(t)
	sealed, err := app.sealSSHDocument(sshConfigDocument{})
	if err != nil || sealed != "" {
		t.Fatalf("want empty seal, got %q err=%v", sealed, err)
	}
}

func TestValidateSSHDocument(t *testing.T) {
	app := newTestApplication(t)
	cases := []struct {
		name    string
		doc     sshConfigDocument
		driver  string
		wantKey string // "" means expect no error
	}{
		{"disabled ok", sshConfigDocument{Enabled: false}, "postgres", ""},
		{"unsupported driver", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "password", Password: "p", InsecureSkipHostKey: true}, "sqlite", "ssh"},
		{"missing host", sshConfigDocument{Enabled: true, User: "u", AuthMethod: "password", Password: "p", InsecureSkipHostKey: true}, "postgres", "ssh"},
		{"missing user", sshConfigDocument{Enabled: true, Host: "h", AuthMethod: "password", Password: "p", InsecureSkipHostKey: true}, "postgres", "ssh"},
		{"bad auth method", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "kerberos", InsecureSkipHostKey: true}, "postgres", "ssh"},
		{"password method needs password", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "password", InsecureSkipHostKey: true}, "postgres", "ssh"},
		{"key method needs key", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "private_key", InsecureSkipHostKey: true}, "postgres", "ssh"},
		{"bad pem", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "private_key", PrivateKeyPEM: "not a key", InsecureSkipHostKey: true}, "postgres", "ssh"},
		{"no host key material", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "password", Password: "p"}, "postgres", "ssh"},
		{"valid with fingerprint", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "password", Password: "p", Fingerprint: "SHA256:abc"}, "postgres", ""},
		{"valid with insecure optout", sshConfigDocument{Enabled: true, Host: "h", User: "u", AuthMethod: "password", Password: "p", InsecureSkipHostKey: true}, "postgres", ""},
		{"port out of range", sshConfigDocument{Enabled: true, Host: "h", User: "u", Port: 99999, AuthMethod: "password", Password: "p", InsecureSkipHostKey: true}, "postgres", "ssh"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := validator.Validator{}
			app.validateSSHDocument(tc.driver, tc.doc, &v)
			_, has := v.FieldErrors[tc.wantKey]
			if tc.wantKey == "" && v.HasErrors() {
				t.Fatalf("want no error, got %v", v.FieldErrors)
			}
			if tc.wantKey != "" && !has {
				t.Fatalf("want field error %q, got %v", tc.wantKey, v.FieldErrors)
			}
		})
	}
}
