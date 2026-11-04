package httpserver

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const (
	// maxForwardedHops membatasi hop X-Forwarded-For yang dinilai (dari kanan); sisa di kiri diabaikan.
	maxForwardedHops = 32
	// maxForwardedBytes membatasi panjang gabungan header X-Forwarded-For; lebih panjang dianggap cacat.
	maxForwardedBytes = 4096
)

// resolveClientIP menentukan IP klien (PRD-001 Q26, ERP_TRUSTED_PROXIES).
//
//   - Peer TCP (r.RemoteAddr) di luar daftar tepercaya, atau daftar kosong: peer; X-Forwarded-For diabaikan
//     sepenuhnya (klien tidak dapat memalsukannya).
//   - Peer tepercaya: X-Forwarded-For (semua barisnya digabung berurutan) dibaca dari KANAN ke KIRI dan hop pertama
//     yang bukan proxy tepercaya adalah klien. Hop di kiri hop itu adalah klaim klien dan tidak berpengaruh.
//   - Peer dipakai bila semua hop tepercaya, header kosong, atau header cacat: hop kosong, bukan IP, memuat port atau
//     zona IPv6, header lebih dari maxForwardedBytes. Hanya maxForwardedHops hop terkanan yang dinilai.
//
// X-Real-IP tidak dipakai. Hasil adalah IP kanonik (IPv4-mapped di-unmap); "" bila peer tidak terurai.
func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	peer = peer.Unmap().WithZone("")
	if len(trusted) == 0 || !isTrusted(peer, trusted) {
		return peer.String()
	}
	if ip, ok := forwardedClient(r.Header.Values("X-Forwarded-For"), trusted); ok {
		return ip.String()
	}
	return peer.String()
}

// forwardedClient mengembalikan hop pertama non-tepercaya dari kanan; ok false bila tidak ada atau header cacat.
func forwardedClient(values []string, trusted []netip.Prefix) (netip.Addr, bool) {
	if len(values) == 0 {
		return netip.Addr{}, false
	}
	joined := strings.Join(values, ",")
	if len(joined) > maxForwardedBytes {
		return netip.Addr{}, false
	}
	hops := strings.Split(joined, ",")
	if len(hops) > maxForwardedHops {
		hops = hops[len(hops)-maxForwardedHops:]
	}
	addrs := make([]netip.Addr, len(hops))
	for i, h := range hops {
		a, err := netip.ParseAddr(strings.TrimSpace(h))
		if err != nil || a.Zone() != "" {
			return netip.Addr{}, false // seluruh header tidak dipercaya bila ada satu hop cacat
		}
		addrs[i] = a.Unmap()
	}
	for i := len(addrs) - 1; i >= 0; i-- {
		if !isTrusted(addrs[i], trusted) {
			return addrs[i], true
		}
	}
	return netip.Addr{}, false
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
