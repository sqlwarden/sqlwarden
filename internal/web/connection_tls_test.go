package web

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func TestTLSConfigDocumentRoundTrip(t *testing.T) {
	app := newTestApplication(t)

	empty := tlsConfigDocument{Mode: "disable"}
	if !empty.isEmpty() {
		t.Fatal("disable + blank should be empty")
	}
	sealed, err := app.sealTLSDocument(empty)
	if err != nil || sealed != "" {
		t.Fatalf("empty seal: %q %v", sealed, err)
	}

	doc := tlsConfigDocument{
		Mode:       "verify-full",
		ServerName: "db.internal",
		CAPEM:      "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----",
	}
	sealed, err = app.sealTLSDocument(doc)
	if err != nil || sealed == "" {
		t.Fatalf("seal: %q %v", sealed, err)
	}
	got, has, err := app.decodeTLSDocument(sealed)
	if err != nil || !has {
		t.Fatalf("decode: has=%v err=%v", has, err)
	}
	if got.Mode != "verify-full" || got.ServerName != "db.internal" || got.CAPEM != doc.CAPEM {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	eng := doc.toEngine()
	if eng == nil || eng.Mode != engine.TLSModeVerifyFull {
		t.Fatalf("toEngine: %+v", eng)
	}
}

func TestTLSConfigDocumentDecodeEmpty(t *testing.T) {
	app := newTestApplication(t)
	_, has, err := app.decodeTLSDocument("")
	if err != nil || has {
		t.Fatalf("empty decode: has=%v err=%v", has, err)
	}
}
