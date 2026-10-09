package credentials_test

import (
	"testing"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/encrypt"
)

func TestSSHDocumentRoundTrip(t *testing.T) {
	kr, err := encrypt.NewKeyring("test-encryption-key-32bytes!!!!!")
	if err != nil {
		t.Fatal(err)
	}
	in := credentials.SSHDocument{
		Enabled: true, Host: "bastion", Port: 2222, User: "ops",
		AuthMethod: "private_key", PrivateKeyPEM: "pem", Passphrase: "pp",
		KnownHostsEntry: "kh", Fingerprint: "SHA256:abc", InsecureSkipHostKey: true,
	}
	sealed, err := credentials.EncodeSSHDocument(kr, in)
	if err != nil {
		t.Fatal(err)
	}
	out, has, err := credentials.DecodeSSHDocument(kr, sealed)
	if err != nil || !has {
		t.Fatalf("decode: has=%v err=%v", has, err)
	}
	if out != in {
		t.Fatalf("round trip mismatch")
	}
	cfg, err := credentials.DecodeSSHConfig(kr, sealed)
	if err != nil || cfg == nil || cfg.Host != "bastion" || cfg.Passphrase != "pp" || cfg.AuthMethod != credentials.SSHAuthPrivateKey {
		t.Fatalf("config decode failed: %v", err)
	}
	empty, err := credentials.EncodeSSHDocument(kr, credentials.SSHDocument{})
	if err != nil || empty != "" {
		t.Fatalf("empty document sealed to %q, %v", empty, err)
	}
}
