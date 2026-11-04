package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"strings"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

// UnknownIP adalah kunci bersama untuk alamat yang tidak dapat diurai (gagal tertutup: semuanya berbagi satu bucket,
// bukan lolos tanpa batas).
const UnknownIP = "unknown"

// NormalizeIP adalah satu-satunya bentuk kanonik IP klien untuk batas laju, jeda lockout, dan hitungan lintas IP:
// IPv4 apa adanya; IPv4-mapped IPv6 di-unmap menjadi IPv4; IPv6 diwakili prefix /64-nya ("2001:db8:1:2::/64") karena
// satu pelanggan biasanya menerima seluruh /64, sehingga penyerang tidak dapat memperoleh bucket/pasangan baru atau
// "IP berbeda" palsu dengan berputar di dalam satu /64; zona dan port dibuang. Masukan yang tidak dapat diurai menjadi
// UnknownIP. Hasilnya paling panjang 24 karakter (kolom login_attempts.ip memuat sampai 45).
func NormalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	a, err := netip.ParseAddr(ip)
	if err != nil {
		ap, err2 := netip.ParseAddrPort(ip)
		if err2 != nil {
			return canonicalPrefix(ip)
		}
		a = ap.Addr()
	}
	a = a.Unmap().WithZone("")
	if a.Is6() {
		p, err := a.Prefix(64)
		if err != nil {
			return UnknownIP
		}
		return p.Masked().String()
	}
	return a.String()
}

// canonicalPrefix membuat NormalizeIP idempoten: keluarannya sendiri ("2001:db8:1:2::/64") diterima apa adanya, tetapi
// HANYA prefix IPv6 /64 yang sudah ter-mask (bit host nol, bukan IPv4-mapped); bentuk prefix lain atau cacat menjadi
// UnknownIP. Kunci pasangan (akun, IP) wajib sama di semua jalur (sandi, sukses, MFA, pelacak email tak dikenal).
func canonicalPrefix(s string) string {
	p, err := netip.ParsePrefix(s)
	if err != nil || p.Bits() != 64 || !p.Addr().Is6() || p.Addr().Is4In6() || p.Addr().Zone() != "" || p != p.Masked() {
		return UnknownIP
	}
	return p.String()
}

// TargetKey membentuk kunci PrincipalLimiter untuk identitas sasaran pada endpoint publik: lingkup (mis. "login",
// "forgot", "refresh"), tenant, dan nilai sasaran (email ternormalisasi, family). Nilai di-hash (SHA-256, 128 bit
// pertama) sehingga peta batas laju tidak memuat data pribadi dan panjang kunci terbatas.
func TargetKey(scope string, tenant tenancy.TenantID, value string) string {
	h := sha256.Sum256([]byte(tenant.String() + "\x00" + value))
	return scope + ":" + hex.EncodeToString(h[:16])
}

// EmailTargetKey adalah TargetKey untuk email: dinormalisasi lewat store.NormalizeEmail (trim + huruf kecil), bentuk
// kanonik yang sama dengan pencarian akun.
func EmailTargetKey(scope string, tenant tenancy.TenantID, email string) string {
	return TargetKey(scope, tenant, store.NormalizeEmail(email))
}
