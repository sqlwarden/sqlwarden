// Package clientip resolves the client address of an HTTP request.
// X-Forwarded-For is believed only while each hop is a trusted proxy.
package clientip

import (
	"net"
	"net/netip"
	"strings"
)

// Resolver resolves client addresses through trusted proxies.
type Resolver struct {
	trusted []netip.Prefix
}

// New creates a resolver with the supplied trusted proxy networks. It
// rewrites IPv4-mapped IPv6 prefixes as IPv4, because Resolve unmaps every
// address before it checks trust.
func New(trusted []netip.Prefix) Resolver {
	normalized := make([]netip.Prefix, 0, len(trusted))
	for _, p := range trusted {
		if p.Addr().Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96).Masked()
		}
		normalized = append(normalized, p)
	}
	return Resolver{trusted: normalized}
}

// Resolve returns the first address, walking from the direct peer leftward
// through X-Forwarded-For, that is not a trusted proxy. An unparsable hop
// ends the walk at the last trusted address. Without a trusted peer the
// header is ignored.
func (r Resolver) Resolve(remoteAddr string, forwardedFor []string) netip.Addr {
	peer := parseHost(remoteAddr)
	if !peer.IsValid() || !r.isTrusted(peer) {
		return peer
	}
	hops := splitHops(forwardedFor)
	current := peer
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(hops[i])
		if err != nil {
			return current
		}
		addr = addr.Unmap()
		if !r.isTrusted(addr) {
			return addr
		}
		current = addr
	}
	return current
}

func (r Resolver) isTrusted(addr netip.Addr) bool {
	for _, p := range r.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

func parseHost(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func splitHops(values []string) []string {
	var hops []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	return hops
}
