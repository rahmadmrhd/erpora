package identity

import (
	"net/http"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/mfa"
	"erpora/internal/platform/tenancy"
	"erpora/sdk/httpx"
	sdkidentity "erpora/sdk/identity"
)

// IPLimit membuat middleware batas laju per IP klien untuk endpoint auth/token (ADR-0013 §4): setelah tenantContext
// dan SEBELUM Authenticate, sehingga penebak token atau kredensial tidak mendapat jawaban dari Authenticate setelah
// jatah IP-nya habis. IP berasal dari httpx.ClientIP (alamat peer; lihat httpserver). Melebihi batas: 429
// `rate_limited` + Retry-After bilangan bulat (httpx.TooManyRequests), tanpa membedakan sebab.
func IPLimit(l *guard.IPLimiter) func(http.Handler) http.Handler {
	if l == nil {
		panic("identity: IPLimit memerlukan IPLimiter")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := httpx.ClientIP(r.Context())
			if ok, retry := l.Allow(ip); !ok {
				httpx.TooManyRequests(w, r, retry)
				return
			}
			next.ServeHTTP(w, r.WithContext(mfa.WithClientIP(r.Context(), ip)))
		})
	}
}

// PrincipalLimit membuat middleware batas laju per principal (klaim sub, per tenant) untuk endpoint auth terautentikasi
// (change, mfa/*, logout), SETELAH Authenticate. Tanpa Principal (middleware dipasang di rute yang tidak
// terautentikasi) request ditolak 401 agar pembatas tidak pernah berjalan dengan kunci kosong.
func PrincipalLimit(l *guard.PrincipalLimiter) func(http.Handler) http.Handler {
	if l == nil {
		panic("identity: PrincipalLimit memerlukan PrincipalLimiter")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := sdkidentity.FromContext(r.Context())
			if !ok || p.UserID == "" {
				reject(w, r, codeUnauthenticated)
				return
			}
			t, _ := tenancy.FromContext(r.Context())
			if ok, retry := l.Allow(guard.TargetKey("principal", t, p.UserID)); !ok {
				httpx.TooManyRequests(w, r, retry)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
