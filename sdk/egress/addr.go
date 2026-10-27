package egress

import (
	"net/netip"
)

type blockedPrefix struct {
	prefix netip.Prefix
	reason string
}

func mustPrefix(s, reason string) blockedPrefix {
	return blockedPrefix{netip.MustParsePrefix(s), reason}
}

// blockedPrefixes adalah rentang yang selalu ditolak (security-guidelines §3.3).
var blockedPrefixes = []blockedPrefix{
	mustPrefix("0.0.0.0/8", "unspecified"),
	mustPrefix("10.0.0.0/8", "private"),
	mustPrefix("100.64.0.0/10", "cgnat"),
	mustPrefix("127.0.0.0/8", "loopback"),
	mustPrefix("169.254.0.0/16", "link_local"),
	mustPrefix("172.16.0.0/12", "private"),
	mustPrefix("192.0.0.0/24", "reserved"),
	mustPrefix("192.168.0.0/16", "private"),
	mustPrefix("198.18.0.0/15", "reserved"),
	mustPrefix("224.0.0.0/4", "multicast"),
	mustPrefix("240.0.0.0/4", "reserved"), // termasuk 255.255.255.255
	mustPrefix("::/128", "unspecified"),
	mustPrefix("::1/128", "loopback"),
	mustPrefix("fc00::/7", "unique_local"),
	mustPrefix("fe80::/10", "link_local"),
	mustPrefix("fec0::/10", "reserved"),
	mustPrefix("ff00::/8", "multicast"),
	// Tambahan Q32 M-1 (security-guidelines §3.3): rentang yang dapat memetakan ke IPv4 internal atau tidak routable.
	mustPrefix("::/96", "ipv4_compatible"),          // IPv4-compatible (usang)
	mustPrefix("::ffff:0:0:0/96", "siit"),           // SIIT (RFC 2765); IPv4-mapped ::ffff:0:0/96 di-Unmap lebih dulu
	mustPrefix("64:ff9b:1::/48", "nat64_local_use"), // NAT64 local-use (RFC 8215)
	mustPrefix("2001::/32", "teredo"),
	mustPrefix("100::/64", "discard_only"),
	mustPrefix("192.0.2.0/24", "documentation"),
	mustPrefix("198.51.100.0/24", "documentation"),
	mustPrefix("203.0.113.0/24", "documentation"),
	mustPrefix("2001:db8::/32", "documentation"),
}

var (
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour   = netip.MustParsePrefix("2002::/16")
)

// blockedReason melaporkan apakah a (setelah Unmap) berada di rentang terblokir, termasuk IPv4 yang
// tertanam pada NAT64 (64:ff9b::/96) dan 6to4 (2002::/16).
func blockedReason(a netip.Addr) (reason string, blocked bool) {
	a = a.Unmap()
	if !a.IsValid() {
		return "invalid", true
	}
	for _, bp := range blockedPrefixes {
		if bp.prefix.Contains(a) {
			return bp.reason, true
		}
	}
	switch {
	case nat64Prefix.Contains(a):
		b := a.As16()
		if _, bad := blockedReason(netip.AddrFrom4([4]byte(b[12:16]))); bad {
			return "nat64", true
		}
	case sixToFour.Contains(a):
		b := a.As16()
		if _, bad := blockedReason(netip.AddrFrom4([4]byte(b[2:6]))); bad {
			return "6to4", true
		}
	}
	return "", false
}
