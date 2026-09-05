package web

import (
	"testing"

	"github.com/sqlwarden/internal/engine"
)

func TestTLSConfigDocumentRoundTrip(t *testing.T) {
	app := newTestApplication(t)

	blank := tlsConfigDocument{}
	if !blank.isEmpty() {
		t.Fatal("blank mode should be empty")
	}
	sealed, err := app.sealTLSDocument(blank)
	if err != nil || sealed != "" {
		t.Fatalf("empty seal: %q %v", sealed, err)
	}

	disable := tlsConfigDocument{Mode: "disable"}
	if disable.isEmpty() {
		t.Fatal("explicit disable should not be empty")
	}
	sealed, err = app.sealTLSDocument(disable)
	if err != nil || sealed == "" {
		t.Fatalf("disable seal: %q %v", sealed, err)
	}
	got, has, err := app.decodeTLSDocument(sealed)
	if err != nil || !has || got.Mode != "disable" {
		t.Fatalf("disable round trip: has=%v err=%v got=%+v", has, err, got)
	}
	eng := disable.toEngine()
	if eng == nil || eng.Mode != engine.TLSModeDisable {
		t.Fatalf("disable toEngine: %+v", eng)
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
	got, has, err = app.decodeTLSDocument(sealed)
	if err != nil || !has {
		t.Fatalf("decode: has=%v err=%v", has, err)
	}
	if got.Mode != "verify-full" || got.ServerName != "db.internal" || got.CAPEM != doc.CAPEM {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	eng = doc.toEngine()
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
