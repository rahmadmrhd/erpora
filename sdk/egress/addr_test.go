package egress

import (
	"net/netip"
	"testing"
)

func TestBlockedReason_Table(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.255.255.254", "::1", "10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1",
		"169.254.169.254", "fe80::1", "fc00::1", "fd12::1", "100.64.0.1", "0.0.0.0", "224.0.0.1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.0.0.1", "::", "255.255.255.255", "240.0.0.1", "198.18.0.1", "192.0.0.1",
		"64:ff9b::a00:1", "2002:a00:1::",
	}
	for _, s := range blocked {
		if reason, bad := blockedReason(netip.MustParseAddr(s)); !bad || reason == "" {
			t.Errorf("%s harus terblokir (reason=%q bad=%v)", s, reason, bad)
		}
	}
	allowed := []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111", "172.32.0.1", "100.128.0.1", "64:ff9b::5db8:d822"}
	for _, s := range allowed {
		if reason, bad := blockedReason(netip.MustParseAddr(s)); bad {
			t.Errorf("%s harus diizinkan, terblokir (%s)", s, reason)
		}
	}
}
