package module

const (
	// DefaultMaxBodyBytes adalah batas body request default per route (1 MiB).
	DefaultMaxBodyBytes int64 = 1 << 20
	// MaxRouteBodyBytes adalah batas atas yang boleh diminta lewat MaxBodyBytes (10 MiB).
	MaxRouteBodyBytes int64 = 10 << 20
)

// RouteConfig adalah hasil resolusi RouteOption.
type RouteConfig struct {
	MaxBodyBytes int64 // 0 = default platform (bila MaxBodyBytesSet false)
	// MaxBodyBytesSet true bila MaxBodyBytes(n) dipanggil; n <= 0 yang diberikan eksplisit ditolak registry.
	MaxBodyBytesSet bool
	Classification  Classification // "" = tidak dinyatakan
	// RequireMFA true bila route menuntut sesi yang faktor keduanya sudah diverifikasi (klaim amr memuat otp),
	// apa pun status faktor pengguna (ADR-0013 §4). Hanya memperketat akses.
	RequireMFA bool
}

// RouteOption mengubah RouteConfig pada Registrar.Handle/Public (Q26 butir 13).
type RouteOption func(*RouteConfig)

// MaxBodyBytes menetapkan batas body request route; hanya 1 <= n <= MaxRouteBodyBytes yang valid
// (nilai lain ditolak registry sebagai invalid_route_option).
func MaxBodyBytes(n int64) RouteOption {
	return func(c *RouteConfig) { c.MaxBodyBytes, c.MaxBodyBytesSet = n, true }
}

// Classify menyatakan klasifikasi data respons route; confidential ke atas menghasilkan
// Cache-Control: no-store. Dipanggil berulang → kelas tertinggi. Nilai tidak valid disimpan apa adanya
// dan tidak tergantikan, agar registry melaporkannya sebagai invalid_route_option.
func Classify(c Classification) RouteOption {
	return func(cfg *RouteConfig) {
		if cfg.Classification != "" && !cfg.Classification.Valid() {
			return
		}
		if !c.Valid() {
			cfg.Classification = c
			return
		}
		cfg.Classification = MaxClassification(cfg.Classification, c)
	}
}

// RequireMFA menuntut sesi dengan faktor kedua terverifikasi (amr memuat `otp`) untuk route ini, walau pengguna
// belum punya faktor MFA: tanpa itu platform menjawab 403 `mfa_required` (ADR-0013 §4, FR-107). Opsi ini hanya
// memperketat akses; pelonggaran (route yang boleh dipanggil sesi yang menunggu MFA) bukan opsi modul, melainkan
// daftar platform yang berlaku hanya untuk route auth modul `erp.core`.
func RequireMFA() RouteOption { return func(c *RouteConfig) { c.RequireMFA = true } }

// ResolveRouteOptions menerapkan opsi berurutan; opsi nil diabaikan.
func ResolveRouteOptions(opts ...RouteOption) RouteConfig {
	var cfg RouteConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return cfg
}
