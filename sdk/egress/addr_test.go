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
		// M-1 (Q32): IPv4-compatible, SIIT, NAT64 local-use, Teredo, discard-only, dan rentang dokumentasi.
		"fe80::1%eth0", "2606:4700:4700::1111%eth0", "::1%lo", "::ffff:127.0.0.1%eth0",
		"::1.2.3.4", "::102:304", "::ffff:0:1.2.3.4", "::ffff:0:102:304", "64:ff9b:1::1", "64:ff9b:1:ffff::1", "2001::1", "2001:0:4136:e378:8000:63bf:3fff:fdd2",
		"100::1", "100::ffff:ffff:ffff:ffff", "192.0.2.1", "192.0.2.255", "198.51.100.7", "203.0.113.200", "2001:db8::1", "2001:db8:ffff::1", "::ffff:192.0.2.1",
	}
	for _, s := range blocked {
		if reason, bad := blockedReason(netip.MustParseAddr(s)); !bad || reason == "" {
			t.Errorf("%s harus terblokir (reason=%q bad=%v)", s, reason, bad)
		}
	}
	allowed := []string{"93.184.216.34", "1.1.1.1", "2606:4700:4700::1111", "172.32.0.1", "100.128.0.1", "64:ff9b::5db8:d822",
		// Tetangga rentang baru yang tidak boleh ikut terblokir.
		"2001:4860:4860::8888", "2001:1::1", "2001:db9::1", "100:0:0:1::1", "192.0.3.1", "198.51.101.1", "203.0.114.1", "64:ff9b:2::1"}
	for _, s := range allowed {
		if reason, bad := blockedReason(netip.MustParseAddr(s)); bad {
			t.Errorf("%s harus diizinkan, terblokir (%s)", s, reason)
		}
	}
}
