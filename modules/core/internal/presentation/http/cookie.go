package httpapi

import (
	"net/http"
	"time"
)

const (
	// RefreshCookieName adalah nama cookie refresh token. Nilainya hanya sampai ke klien lewat cookie ini (ADR-0013 §1):
	// tidak pernah di body respons, dan tidak terbaca skrip karena HttpOnly.
	RefreshCookieName = "erp_refresh"
	// RefreshCookiePath membatasi cookie ke endpoint refresh sehingga tidak ikut ke request API lain.
	RefreshCookiePath = "/api/core/auth/refresh"
)

// setRefreshCookie menulis (atau menulis ulang) cookie refresh: HttpOnly, Secure, SameSite=Strict, Path khusus
// endpoint refresh, dan umur sampai expires (kedaluwarsa geser token; PRD-001 FR-101, ADR-0013 §1). Dipanggil pada login,
// refresh, serta mfa/confirm, mfa/verify, dan mfa/recovery yang merotasi token (Q27). Nilai token tidak pernah dicatat.
func setRefreshCookie(w http.ResponseWriter, value string, expires, now time.Time) {
	maxAge := int(expires.Sub(now) / time.Second)
	if maxAge < 1 {
		maxAge = 1
	}
	http.SetCookie(w, &http.Cookie{
		Name: RefreshCookieName, Value: value, Path: RefreshCookiePath,
		Expires: expires.UTC(), MaxAge: maxAge,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie menghapus cookie refresh (logout, atau refresh token ditolak) dengan atribut yang sama agar peramban
// mencocokkan cookie yang sama.
func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: RefreshCookieName, Value: "", Path: RefreshCookiePath,
		Expires: time.Unix(0, 0).UTC(), MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

// refreshCookie membaca nilai cookie refresh; "" bila tidak ada.
func refreshCookie(r *http.Request) string {
	c, err := r.Cookie(RefreshCookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
