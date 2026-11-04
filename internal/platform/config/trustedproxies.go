package config

import (
	"net/netip"
	"strconv"
	"strings"
)

// MaxTrustedProxies adalah jumlah entri maksimum ERP_TRUSTED_PROXIES (daftar proxy sungguhan pendek; batas ini
// mencegah daftar raksasa yang memperlambat pencocokan pada setiap request).
const MaxTrustedProxies = 64

// loadTrustedProxies mengurai ERP_TRUSTED_PROXIES (PRD-001 Q26): daftar IP atau CIDR dipisah koma; spasi hanya
// boleh di sekitar koma. Kosong = tanpa proxy tepercaya (X-Forwarded-For tidak pernah dipercaya). Ditolak: entri
// kosong, spasi atau karakter kontrol di dalam entri, hostname, port, zona IPv6, prefiks /0 (mempercayai seluruh
// internet berarti klien dapat memalsukan IP-nya), dan lebih dari MaxTrustedProxies entri. IP tunggal menjadi
// /32 atau /128; CIDR dinormalisasi ke alamat jaringannya; IPv4-mapped IPv6 di-unmap. Pesan galat tidak
// mengulang masukan.
func loadTrustedProxies(c *Config, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	bad := func(why string) error { return ValidationError{"ERP_TRUSTED_PROXIES", why} }
	parts := strings.Split(raw, ",")
	if len(parts) > MaxTrustedProxies {
		return bad("terlalu banyak entri (maksimum 64)")
	}
	out := make([]netip.Prefix, 0, len(parts))
	for i, part := range parts {
		e := strings.TrimSpace(part)
		if e == "" {
			return bad("entri kosong (periksa koma ganda atau koma di ujung)")
		}
		if strings.IndexFunc(e, func(r rune) bool { return r <= ' ' || r == 0x7f }) >= 0 {
			return bad("entri tidak boleh memuat spasi atau karakter kontrol (pisahkan entri dengan koma)")
		}
		if strings.Contains(e, "%") {
			return bad("zona IPv6 tidak diizinkan")
		}
		var p netip.Prefix
		if strings.Contains(e, "/") {
			pp, err := netip.ParsePrefix(e)
			if err != nil {
				return bad("entri ke-" + strconv.Itoa(i+1) + " bukan CIDR yang sah")
			}
			p = pp
		} else {
			a, err := netip.ParseAddr(e)
			if err != nil {
				return bad("entri ke-" + strconv.Itoa(i+1) + " bukan IP atau CIDR yang sah (hostname dan port tidak diizinkan)")
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		if p.Bits() == 0 {
			return bad("prefiks /0 ditolak: mempercayai semua alamat membuat IP klien dapat dipalsukan")
		}
		// IPv4-mapped (::ffff:a.b.c.d/n) menjadi IPv4 agar cocok dengan alamat peer yang di-unmap.
		if a := p.Addr(); a.Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(a.Unmap(), p.Bits()-96)
			if p.Bits() == 0 {
				return bad("prefiks /0 ditolak: mempercayai semua alamat membuat IP klien dapat dipalsukan")
			}
		}
		out = append(out, p.Masked())
	}
	c.TrustedProxies = out
	return nil
}
