package clientip

import (
	"net/netip"
	"testing"
)

func mustPrefixes(t *testing.T, s ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, len(s))
	for i, v := range s {
		out[i] = netip.MustParsePrefix(v)
	}
	return out
}

func TestResolveUsesPeerWithoutTrustedProxies(t *testing.T) {
	got := New(nil).Resolve("203.0.113.9:5555", []string{"198.51.100.1"})
	if got != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveIgnoresForwardedFromUntrustedPeer(t *testing.T) {
	r := New(mustPrefixes(t, "10.0.0.0/8"))
	got := r.Resolve("203.0.113.9:5555", []string{"198.51.100.1"})
	if got != netip.MustParseAddr("203.0.113.9") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveWalksForwardedRightToLeft(t *testing.T) {
	r := New(mustPrefixes(t, "10.0.0.0/8"))
	got := r.Resolve("10.0.0.2:443", []string{"198.51.100.1, 10.0.0.7", "10.0.0.8"})
	if got != netip.MustParseAddr("198.51.100.1") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveSkipsGarbageAndStopsAtFirstUntrusted(t *testing.T) {
	r := New(mustPrefixes(t, "10.0.0.0/8"))
	got := r.Resolve("10.0.0.2:443", []string{"spoofed, 203.0.113.5, not-an-ip"})
	if got != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveAllTrustedReturnsLeftmost(t *testing.T) {
	r := New(mustPrefixes(t, "10.0.0.0/8"))
	got := r.Resolve("10.0.0.2:443", []string{"10.0.0.9"})
	if got != netip.MustParseAddr("10.0.0.9") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveUnparsablePeer(t *testing.T) {
	if got := New(nil).Resolve("garbage", nil); got.IsValid() {
		t.Fatalf("got %v", got)
	}
}

func TestResolveUnmapsIPv4InIPv6(t *testing.T) {
	got := New(nil).Resolve("[::ffff:192.0.2.1]:80", nil)
	if got != netip.MustParseAddr("192.0.2.1") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveTrustsIPv4MappedPrefix(t *testing.T) {
	r := New(mustPrefixes(t, "::ffff:10.0.0.0/104"))
	got := r.Resolve("10.0.0.2:443", []string{"198.51.100.1"})
	if got != netip.MustParseAddr("198.51.100.1") {
		t.Fatalf("got %v", got)
	}
}

func TestResolveUnmapsForwardedHop(t *testing.T) {
	r := New(mustPrefixes(t, "10.0.0.0/8"))
	got := r.Resolve("10.0.0.2:443", []string{"::ffff:198.51.100.1"})
	if got != netip.MustParseAddr("198.51.100.1") {
		t.Fatalf("got %v", got)
	}
}
