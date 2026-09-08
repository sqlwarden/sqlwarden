package sqlserver

import "testing"

func TestSQLServerTLSSpec(t *testing.T) {
	spec := (&Driver{}).TLSSpec()
	if len(spec.Modes) != 4 || !spec.SupportsClientCert || !spec.SupportsCABundle || !spec.SupportsServerName {
		t.Fatalf("unexpected spec: %+v", spec)
	}
}

func TestSQLServerSupportsSSHTunnel(t *testing.T) {
	if !(&Driver{}).SupportsSSHTunnel() {
		t.Fatal("want SSH tunnel support")
	}
}
